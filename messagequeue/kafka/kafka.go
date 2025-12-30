package kafka

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var (
	mqProduced = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "mq_produced_total", Help: "消息生产总数"},
		[]string{"topic", "status"},
	)
	mqConsumed = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "mq_consumed_total", Help: "消息消费总数"},
		[]string{"topic", "status"},
	)
	mqDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mq_operation_duration_seconds",
			Help:    "MQ操作耗时",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"topic", "operation"},
	)
	mqLag = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mq_consume_lag_seconds",
			Help:    "消息消费延迟",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"topic"},
	)
)

func init() {
	prometheus.MustRegister(mqProduced, mqConsumed, mqDuration, mqLag)
}

type Handler func(ctx context.Context, msg kafkago.Message) error

type Producer struct {
	writer    *kafkago.Writer
	dlqWriter *kafkago.Writer
	logger    *logging.Logger
}

func NewProducer(cfg config.KafkaConfig, logger *logging.Logger) *Producer {
	w := &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafkago.Hash{},
		WriteTimeout: cfg.WriteTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		MaxAttempts:  5,
		RequiredAcks: kafkago.RequireAll,
		Async:        cfg.Async,
	}

	dlqWriter := &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Topic:        cfg.Topic + ".dlq",
		Balancer:     &kafkago.LeastBytes{},
		RequiredAcks: kafkago.RequireOne,
	}

	return &Producer{writer: w, dlqWriter: dlqWriter, logger: logger}
}

func (p *Producer) Publish(ctx context.Context, key, value []byte) error {
	start := time.Now()
	tracer := otel.Tracer("kafka-producer")
	ctx, span := tracer.Start(ctx, "Kafka.Publish", trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()

	headers := make([]kafkago.Header, 0)
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		headers = append(headers, kafkago.Header{Key: k, Value: []byte(v)})
	}

	msg := kafkago.Message{
		Key:     key,
		Value:   value,
		Headers: headers,
		Time:    time.Now(),
	}

	err := p.writer.WriteMessages(ctx, msg)
	mqDuration.WithLabelValues(p.writer.Topic, "publish").Observe(time.Since(start).Seconds())

	if err != nil {
		mqProduced.WithLabelValues(p.writer.Topic, "failed").Inc()
		p.logger.ErrorContext(ctx, "failed to publish message", "error", err)
		if dlqErr := p.dlqWriter.WriteMessages(ctx, msg); dlqErr != nil {
			p.logger.ErrorContext(ctx, "failed to write to DLQ", "error", dlqErr)
		}
		return err
	}

	mqProduced.WithLabelValues(p.writer.Topic, "success").Inc()
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
	reader *kafkago.Reader
	logger *logging.Logger
}

func NewConsumer(cfg config.KafkaConfig, logger *logging.Logger) *Consumer {
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		MinBytes:       10e3,
		MaxBytes:       10e6,
		MaxWait:        time.Second,
		CommitInterval: 0,
	})
	return &Consumer{reader: r, logger: logger}
}

func (c *Consumer) Consume(ctx context.Context, handler Handler) error {
	tracer := otel.Tracer("kafka-consumer")
	for {
		m, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.logger.Error("failed to fetch message", "error", err)
			continue
		}

		carrier := propagation.MapCarrier{}
		for _, h := range m.Headers {
			carrier[h.Key] = string(h.Value)
		}
		extractedCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)
		spanCtx, span := tracer.Start(extractedCtx, "Kafka.Consume", trace.WithSpanKind(trace.SpanKindConsumer))

		start := time.Now()
		handleErr := handler(spanCtx, m)

		mqDuration.WithLabelValues(m.Topic, "consume").Observe(time.Since(start).Seconds())
		mqLag.WithLabelValues(m.Topic).Observe(time.Since(m.Time).Seconds())

		if handleErr != nil {
			mqConsumed.WithLabelValues(m.Topic, "failed").Inc()
			c.logger.ErrorContext(spanCtx, "message handler failed", "error", handleErr, "topic", m.Topic, "offset", m.Offset)
			span.SetStatus(codes.Error, handleErr.Error())
			span.End()
			continue
		}

		if err := c.reader.CommitMessages(ctx, m); err != nil {
			c.logger.ErrorContext(spanCtx, "failed to commit offset", "error", err)
		}

		mqConsumed.WithLabelValues(m.Topic, "success").Inc()
		span.End()
	}
}

func (c *Consumer) Start(ctx context.Context, workers int, handler Handler) {
	for range workers {
		go func() {
			if err := c.Consume(ctx, handler); err != nil && err != context.Canceled {
				c.logger.Error("consumer exit with error", "error", err)
			}
		}()
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
