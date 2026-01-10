package kafka

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var defaultProducer *Producer

// DefaultProducer 返回全局默认生产者实例.
func DefaultProducer() *Producer {
	return defaultProducer
}

// SetDefaultProducer 设置全局默认生产者实例.
func SetDefaultProducer(producer *Producer) {
	defaultProducer = producer
}

// Handler 定义了消息处理函数的原型.
type Handler func(ctx context.Context, msg kafkago.Message) error

// Producer 封装了 Kafka 消息生产者.
type Producer struct {
	writer        *kafkago.Writer
	dlqWriter     *kafkago.Writer
	logger        *logging.Logger
	metrics       *metrics.Metrics
	producedTotal *prometheus.CounterVec
	duration      *prometheus.HistogramVec
}

// NewProducer 初始化并返回一个功能增强的 Kafka 生产者.
func NewProducer(cfg *config.KafkaConfig, logger *logging.Logger, m *metrics.Metrics) *Producer {
	w := &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafkago.Hash{Hasher: nil},
		WriteTimeout: cfg.WriteTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		MaxAttempts:  5,
		RequiredAcks: kafkago.RequireAll,
		Async:        cfg.Async,
	}

	dlqTopic := cfg.Topic
	if dlqTopic == "" {
		dlqTopic = "default"
	}

	dlqWriter := &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Topic:        dlqTopic + ".dlq",
		Balancer:     &kafkago.LeastBytes{},
		RequiredAcks: kafkago.RequireOne,
	}

	producedTotal := m.NewCounterVec(&prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "mq",
		Name:      "produced_total",
		Help:      "Total number of messages produced",
	}, []string{"topic", "status"})

	duration := m.NewHistogramVec(&prometheus.HistogramOpts{
		Namespace: "pkg",
		Subsystem: "mq",
		Name:      "producer_duration_seconds",
		Help:      "Duration of message producing",
		Buckets:   prometheus.DefBuckets,
	}, []string{"topic"})

	return &Producer{
		writer:        w,
		dlqWriter:     dlqWriter,
		logger:        logger,
		metrics:       m,
		producedTotal: producedTotal,
		duration:      duration,
	}
}

// Publish 将消息发送至默认配置的主题.
func (p *Producer) Publish(ctx context.Context, key, value []byte) error {
	return p.PublishToTopic(ctx, p.writer.Topic, key, value)
}

// PublishToTopic 将消息发送至指定主题.
func (p *Producer) PublishToTopic(ctx context.Context, topic string, key, value []byte) error {
	start := time.Now()
	ctx, span := tracing.Tracer().Start(ctx, "kafka.publish", trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()

	headers := make([]kafkago.Header, 0)
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		headers = append(headers, kafkago.Header{Key: k, Value: []byte(v)})
	}

	msg := kafkago.Message{
		Topic:   topic,
		Key:     key,
		Value:   value,
		Headers: headers,
		Time:    time.Now(),
	}

	err := p.writer.WriteMessages(ctx, msg)
	p.duration.WithLabelValues(topic).Observe(time.Since(start).Seconds())

	if err != nil {
		p.producedTotal.WithLabelValues(topic, "failed").Inc()
		tracing.SetError(ctx, err)
		p.logger.ErrorContext(ctx, "failed to publish message", "topic", topic, "error", err)

		if dlqErr := p.dlqWriter.WriteMessages(ctx, msg); dlqErr != nil {
			p.logger.ErrorContext(ctx, "failed to write to dlq", "error", dlqErr)
		}

		return fmt.Errorf("failed to write messages: %w", err)
	}

	p.producedTotal.WithLabelValues(topic, "success").Inc()

	return nil
}

// Close 优雅关闭主写入器及死信队列写入器.
func (p *Producer) Close() error {
	var errs []error
	if dlqErr := p.dlqWriter.Close(); dlqErr != nil {
		p.logger.Error("failed to close dlq writer", "error", dlqErr)
		errs = append(errs, dlqErr)
	}

	if wErr := p.writer.Close(); wErr != nil {
		p.logger.Error("failed to close writer", "error", wErr)
		errs = append(errs, wErr)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close producers: %w", errors.Join(errs...))
	}

	return nil
}

// Consumer 封装了 Kafka 消息消费者.
type Consumer struct {
	reader        *kafkago.Reader
	logger        *logging.Logger
	metrics       *metrics.Metrics
	consumedTotal *prometheus.CounterVec
	consumeLag    *prometheus.HistogramVec
}

// NewConsumer 初始化并返回一个功能增强的 Kafka 消费者.
func NewConsumer(cfg *config.KafkaConfig, logger *logging.Logger, m *metrics.Metrics) *Consumer {
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		MinBytes:       10e3,
		MaxBytes:       10e6,
		MaxWait:        time.Second,
		CommitInterval: 0,
	})

	consumedTotal := m.NewCounterVec(&prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "mq",
		Name:      "consumed_total",
		Help:      "Total number of messages consumed",
	}, []string{"topic", "status"})

	consumeLag := m.NewHistogramVec(&prometheus.HistogramOpts{
		Namespace: "pkg",
		Subsystem: "mq",
		Name:      "consume_lag_seconds",
		Help:      "Lag of message consuming",
		Buckets:   []float64{0.1, 0.5, 1, 5, 10},
	}, []string{"topic"})

	return &Consumer{
		reader:        r,
		logger:        logger,
		metrics:       m,
		consumedTotal: consumedTotal,
		consumeLag:    consumeLag,
	}
}

// Consume 循环读取并处理消息.
func (c *Consumer) Consume(ctx context.Context, handler Handler) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("context cancelled during fetch: %w", ctx.Err())
			}

			c.logger.Error("failed to fetch message", "error", err)

			continue
		}

		carrier := propagation.MapCarrier{}
		for _, h := range msg.Headers {
			carrier[h.Key] = string(h.Value)
		}

		extractedCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)
		spanCtx, span := tracing.Tracer().Start(extractedCtx, "Kafka.Consume", trace.WithSpanKind(trace.SpanKindConsumer))

		handleErr := handler(spanCtx, msg)
		c.consumeLag.WithLabelValues(msg.Topic).Observe(time.Since(msg.Time).Seconds())

		if handleErr != nil {
			c.consumedTotal.WithLabelValues(msg.Topic, "failed").Inc()
			tracing.SetError(spanCtx, handleErr)
			c.logger.ErrorContext(spanCtx, "message handler failed", "error", handleErr, "topic", msg.Topic)
			span.End()

			continue
		}

		if errCommit := c.reader.CommitMessages(ctx, msg); errCommit != nil {
			c.logger.ErrorContext(spanCtx, "commit failed", "error", errCommit)
		}

		c.consumedTotal.WithLabelValues(msg.Topic, "success").Inc()
		span.End()
	}
}

// Start 并发启动多个消费者协程.
func (c *Consumer) Start(ctx context.Context, workers int, handler Handler) {
	for range workers {
		go func() {
			if err := c.Consume(ctx, handler); err != nil && !errors.Is(err, context.Canceled) {
				c.logger.Error("consumer stopped", "error", err)
			}
		}()
	}
}

// Close 关闭消费者.
func (c *Consumer) Close() error {
	if err := c.reader.Close(); err != nil {
		return fmt.Errorf("failed to close kafka reader: %w", err)
	}

	return nil
}
