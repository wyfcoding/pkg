// Package kafka 提供 Kafka 的生产和消费功能，并集成了 OpenTelemetry 和 Prometheus。
package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var (
	// mqProduced 是一个Prometheus计数器，用于统计生产成功的消息总数。
	mqProduced = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mq_produced_total",
			Help: "The total number of messages produced",
		},
		[]string{"topic", "status"},
	)
	// mqConsumed 是一个Prometheus计数器，用于统计消费成功的消息总数。
	mqConsumed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mq_consumed_total",
			Help: "The total number of messages consumed",
		},
		[]string{"topic", "status"},
	)
	// mqDuration 是一个Prometheus直方图，用于记录MQ操作（生产/消费）的耗时。
	mqDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mq_operation_duration_seconds",
			Help:    "The duration of mq operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"topic", "operation"},
	)
)

func init() {
	prometheus.MustRegister(mqProduced, mqConsumed, mqDuration)
}

// Producer 封装了 Kafka Writer，提供带有 Tracing 和 Metrics 的消息发送功能。
type Producer struct {
	writer    *kafka.Writer
	dlqWriter *kafka.Writer
	logger    *logging.Logger
	tracer    trace.Tracer
}

// NewProducer 创建一个新的 Producer 实例。
func NewProducer(cfg config.KafkaConfig, logger *logging.Logger) *Producer {
	// 初始化 Kafka 写入器，配置地址、主题、平衡策略及超时时间。
	w := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafka.LeastBytes{}, // 优先发送到数据量最小的分区。
		WriteTimeout: cfg.WriteTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		BatchSize:    cfg.MaxBytes,
		BatchTimeout: 10 * time.Millisecond,
		Async:        cfg.Async,
		RequiredAcks: kafka.RequiredAcks(cfg.RequiredAcks),
	}

	// 初始化死信队列（DLQ）写入器，用于处理发送失败的消息。
	dlqWriter := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.Topic + "-dlq",
		Balancer:     &kafka.LeastBytes{},
		WriteTimeout: cfg.WriteTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		Async:        cfg.Async,
		RequiredAcks: kafka.RequiredAcks(cfg.RequiredAcks),
	}

	return &Producer{
		writer:    w,
		dlqWriter: dlqWriter,
		logger:    logger,
		tracer:    otel.Tracer("kafka-producer"),
	}
}

// Publish 发送消息到 Kafka，自动注入 Trace Context，并在失败时重试或写入死信队列。
func (p *Producer) Publish(ctx context.Context, key, value []byte) error {
	start := time.Now()
	// 使用传入的 context 开启 Span
	ctx, span := p.tracer.Start(ctx, "Publish")
	defer span.End()

	defer func() {
		mqDuration.WithLabelValues(p.writer.Topic, "publish").Observe(time.Since(start).Seconds())
	}()

	// 注入 Trace Context 到 Kafka Header
	headers := make([]kafka.Header, 0)
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for k, v := range carrier {
		headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
	}

	msg := kafka.Message{
		Key:     key,
		Value:   value,
		Time:    time.Now(),
		Headers: headers,
	}

	// 简单的重试逻辑
	var err error
	for i := range 3 {
		err = p.writer.WriteMessages(ctx, msg)
		if err == nil {
			mqProduced.WithLabelValues(p.writer.Topic, "success").Inc()
			return nil
		}
		time.Sleep(time.Duration(1<<i) * 100 * time.Millisecond)
	}

	// 发送失败，写入死信队列
	p.logger.ErrorContext(ctx, "failed to publish message, sending to DLQ", "error", err)
	mqProduced.WithLabelValues(p.writer.Topic, "failed").Inc()

	if dlqErr := p.dlqWriter.WriteMessages(ctx, msg); dlqErr != nil {
		p.logger.ErrorContext(ctx, "failed to publish to DLQ", "error", dlqErr)
		return fmt.Errorf("failed to publish to DLQ: %w (original error: %v)", dlqErr, err)
	}

	return nil
}

// Close 关闭 Producer。
func (p *Producer) Close() error {
	if err := p.dlqWriter.Close(); err != nil {
		p.logger.Error("failed to close DLQ writer", "error", err)
	}
	return p.writer.Close()
}

// Consumer 封装了 Kafka Reader，提供带有 Tracing 和 Metrics 的消息消费功能。
type Consumer struct {
	reader *kafka.Reader
	logger *logging.Logger
	tracer trace.Tracer
}

// NewConsumer 创建一个新的 Consumer 实例。
func NewConsumer(cfg config.KafkaConfig, logger *logging.Logger) *Consumer {
	// 初始化 Kafka 读取器，配置消费组、消费方式及偏移量起始点。
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:         cfg.Brokers,
		GroupID:         cfg.GroupID,
		Topic:           cfg.Topic,
		MinBytes:        cfg.MinBytes,
		MaxBytes:        cfg.MaxBytes,
		MaxWait:         cfg.MaxWait,
		ReadLagInterval: -1,
		CommitInterval:  0,                // 设置为0以启用手动提交 Offset。
		StartOffset:     kafka.LastOffset, // 默认从最后一条消息之后开始消费。
	})

	return &Consumer{
		reader: r,
		logger: logger,
		tracer: otel.Tracer("kafka-consumer"),
	}
}

// Consume 持续消费消息并调用 handler。
// 支持 Graceful Shutdown：当 ctx 取消时停止。
func (c *Consumer) Consume(ctx context.Context, handler func(ctx context.Context, msg kafka.Message) error) error {
	for {
		// 检查 Context 是否已取消
		select {
		case <-ctx.Done():
			c.logger.Info("stopping consumer", "topic", c.reader.Config().Topic)
			return ctx.Err()
		default:
		}

		m, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.logger.Error("failed to fetch message", "error", err)
			continue
		}

		// 提取 Trace Context
		carrier := propagation.MapCarrier{}
		for _, h := range m.Headers {
			carrier[h.Key] = string(h.Value)
		}
		parentCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)

		// 开启 Consumer Span
		spanCtx, span := c.tracer.Start(parentCtx, "Consume", trace.WithSpanKind(trace.SpanKindConsumer))

		start := time.Now()
		err = handler(spanCtx, m)
		duration := time.Since(start).Seconds()
		mqDuration.WithLabelValues(c.reader.Config().Topic, "consume").Observe(duration)

		if err != nil {
			span.RecordError(err)
			span.End()
			c.logger.ErrorContext(spanCtx, "handler failed", "error", err)
			mqConsumed.WithLabelValues(c.reader.Config().Topic, "failed").Inc()
			// 消费失败暂不提交 Offset，根据业务需求可增加重试队列逻辑
			continue
		}

		mqConsumed.WithLabelValues(c.reader.Config().Topic, "success").Inc()
		span.End()

		if err := c.reader.CommitMessages(ctx, m); err != nil {
			c.logger.ErrorContext(ctx, "failed to commit message", "error", err)
		}
	}
}

// Start 启动 Consumer (非阻塞，但在后台运行直到 ctx 取消)。
// 注意：原实现是阻塞的，这里保持阻塞语义以便由调用方控制 goroutine。
func (c *Consumer) Start(ctx context.Context, handler func(ctx context.Context, msg kafka.Message) error) error {
	return c.Consume(ctx, handler)
}

// Stop 停止 Consumer。实际上通过取消传递给 Consume 的 Context 来实现，这里仅关闭 Reader 连接。
func (c *Consumer) Stop(ctx context.Context) error {
	return c.reader.Close()
}

// Close 是 Stop 的别名，满足常见的 Closer 接口习惯。
func (c *Consumer) Close() error {
	return c.reader.Close()
}
