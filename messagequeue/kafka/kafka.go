package kafka

import (
	"context"
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

var (
	defaultProducer *Producer
)

// DefaultProducer 返回全局默认生产者实例
func DefaultProducer() *Producer {
	return defaultProducer
}

// SetDefaultProducer 设置全局默认生产者实例
func SetDefaultProducer(p *Producer) {
	defaultProducer = p
}

type Handler func(ctx context.Context, msg kafkago.Message) error

type Producer struct {
	writer    *kafkago.Writer
	dlqWriter *kafkago.Writer
	logger    *logging.Logger
	metrics   *metrics.Metrics

	// 归一化指标
	producedTotal *prometheus.CounterVec
	duration      *prometheus.HistogramVec
}

func NewProducer(cfg config.KafkaConfig, logger *logging.Logger, m *metrics.Metrics) *Producer {
	w := &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Topic:        cfg.Topic, // 如果为空，则必须在 Message 中指定 Topic
		Balancer:     &kafkago.Hash{},
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

	producedTotal := m.NewCounterVec(prometheus.CounterOpts{Name: "mq_produced_total", Help: "消息生产总数"}, []string{"topic", "status"})
	duration := m.NewHistogramVec(prometheus.HistogramOpts{Name: "mq_producer_duration_seconds", Help: "MQ生产耗时"}, []string{"topic"})

	return &Producer{
		writer:        w,
		dlqWriter:     dlqWriter,
		logger:        logger,
		metrics:       m,
		producedTotal: producedTotal,
		duration:      duration,
	}
}

// Publish 发送到默认 Topic
func (p *Producer) Publish(ctx context.Context, key, value []byte) error {
	return p.PublishToTopic(ctx, p.writer.Topic, key, value)
}

// PublishToTopic 发送到指定 Topic
func (p *Producer) PublishToTopic(ctx context.Context, topic string, key, value []byte) error {
	start := time.Now()
	// 1. 使用 pkg/tracing 开启 Span
	ctx, span := tracing.StartSpan(ctx, "Kafka.Publish", trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()

	// 2. 注入 Trace 上下文到消息头
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
			p.logger.ErrorContext(ctx, "failed to write to DLQ", "error", dlqErr)
		}
		return err
	}

	p.producedTotal.WithLabelValues(topic, "success").Inc()
	return nil
}

func (p *Producer) Close() error {
	var err error
	if dlqErr := p.dlqWriter.Close(); dlqErr != nil {
		p.logger.Error("failed to close DLQ writer", "error", dlqErr)
		err = dlqErr
	}
	if wErr := p.writer.Close(); wErr != nil {
		p.logger.Error("failed to close writer", "error", wErr)
		err = wErr
	}
	return err
}

type Consumer struct {
	reader  *kafkago.Reader
	logger  *logging.Logger
	metrics *metrics.Metrics

	consumedTotal *prometheus.CounterVec
	consumeLag    *prometheus.HistogramVec
}

func NewConsumer(cfg config.KafkaConfig, logger *logging.Logger, m *metrics.Metrics) *Consumer {
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		MinBytes:       10e3,
		MaxBytes:       10e6,
		MaxWait:        time.Second,
		CommitInterval: 0,
	})

	consumedTotal := m.NewCounterVec(prometheus.CounterOpts{Name: "mq_consumed_total", Help: "消息消费总数"}, []string{"topic", "status"})
	consumeLag := m.NewHistogramVec(prometheus.HistogramOpts{Name: "mq_consume_lag_seconds", Help: "消息消费延迟", Buckets: []float64{0.1, 0.5, 1, 5, 10}}, []string{"topic"})

	return &Consumer{
		reader:        r,
		logger:        logger,
		metrics:       m,
		consumedTotal: consumedTotal,
		consumeLag:    consumeLag,
	}
}

func (c *Consumer) Consume(ctx context.Context, handler Handler) error {
	for {
		m, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.logger.Error("failed to fetch message", "error", err)
			continue
		}

		// 1. 从消息头提取 Trace 上下文
		carrier := propagation.MapCarrier{}
		for _, h := range m.Headers {
			carrier[h.Key] = string(h.Value)
		}
		extractedCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)

		// 2. 开启消费 Span
		spanCtx, span := tracing.StartSpan(extractedCtx, "Kafka.Consume", trace.WithSpanKind(trace.SpanKindConsumer))

		handleErr := handler(spanCtx, m)
		c.consumeLag.WithLabelValues(m.Topic).Observe(time.Since(m.Time).Seconds())

		if handleErr != nil {
			c.consumedTotal.WithLabelValues(m.Topic, "failed").Inc()
			tracing.SetError(spanCtx, handleErr)
			c.logger.ErrorContext(spanCtx, "message handler failed", "error", handleErr, "topic", m.Topic)
			span.End()
			continue
		}

		if err := c.reader.CommitMessages(ctx, m); err != nil {
			c.logger.ErrorContext(spanCtx, "commit failed", "error", err)
		}

		c.consumedTotal.WithLabelValues(m.Topic, "success").Inc()
		span.End()
	}
}

func (c *Consumer) Start(ctx context.Context, workers int, handler Handler) {
	for range workers {
		go func() {
			if err := c.Consume(ctx, handler); err != nil && err != context.Canceled {
				c.logger.Error("consumer stopped", "error", err)
			}
		}()
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
