package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/idempotency"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/retry"
	"github.com/wyfcoding/pkg/tracing"

	"github.com/prometheus/client_golang/prometheus"
	kafkago "github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var defaultProducer *Producer

const (
	defaultProducerMaxAttempts = 5
	defaultMinBytes            = 10e3
	defaultMaxBytes            = 10e6
	defaultMaxWait             = time.Second
	defaultIdempotencyTTL      = 24 * time.Hour
)

const (
	headerIdempotencyKey    = "x-idempotency-key"
	headerOriginalTopic     = "x-original-topic"
	headerOriginalPartition = "x-original-partition"
	headerOriginalOffset    = "x-original-offset"
	headerError             = "x-error"
)

// DefaultProducer 返回全局默认生产者实例.
func DefaultProducer() *Producer {
	return defaultProducer
}

// SetDefaultProducer 设置全局默认生产者实例.
func SetDefaultProducer(producer *Producer) {
	defaultProducer = producer
}

// RegisterProducerReloadHook 注册生产者热更新回调。
func RegisterProducerReloadHook(producer *Producer) {
	if producer == nil {
		return
	}
	config.RegisterReloadHook(func(updated *config.Config) {
		if updated == nil {
			return
		}
		if err := producer.UpdateConfig(&updated.MessageQueue.Kafka); err != nil {
			logger := producer.logger
			if logger == nil {
				logger = logging.Default()
			}
			logger.Error("kafka producer reload failed", "error", err)
		}
	})
}

// RegisterConsumerReloadHook 注册消费者热更新回调。
func RegisterConsumerReloadHook(consumer *Consumer, opts *ConsumerOptions) {
	if consumer == nil {
		return
	}
	config.RegisterReloadHook(func(updated *config.Config) {
		if updated == nil {
			return
		}
		if err := consumer.UpdateConfig(&updated.MessageQueue.Kafka, opts); err != nil {
			logger := consumer.logger
			if logger == nil {
				logger = logging.Default()
			}
			logger.Error("kafka consumer reload failed", "error", err)
		}
	})
}

// Handler 定义了消息处理函数的原型.
type Handler func(ctx context.Context, msg kafkago.Message) error

// ConsumerOptions 定义 Kafka 消费者的可选治理能力。
type ConsumerOptions struct {
	RetryConfig        retry.Config                     // 重试策略配置。
	Retryable          func(error) bool                 // 判断是否可重试的函数。
	CommitOnError      bool                             // 失败时是否直接提交 offset。
	DLQEnabled         bool                             // 是否开启死信队列投递。
	DLQTopic           string                           // 自定义死信队列主题。
	IdempotencyManager idempotency.Manager              // 幂等管理器。
	IdempotencyTTL     time.Duration                    // 幂等 Key 过期时间。
	IdempotencyKeyFunc func(msg kafkago.Message) string // 自定义幂等 Key 构造逻辑。
}

// Producer 封装了 Kafka 消息生产者.
type Producer struct {
	mu            sync.RWMutex
	writer        *kafkago.Writer
	dlqWriter     *kafkago.Writer
	logger        *logging.Logger
	metrics       *metrics.Metrics
	producedTotal *prometheus.CounterVec
	duration      *prometheus.HistogramVec
}

// NewProducer 初始化并返回一个功能增强的 Kafka 生产者.
func NewProducer(cfg *config.KafkaConfig, logger *logging.Logger, m *metrics.Metrics) *Producer {
	w, dlqWriter := buildProducerWriters(cfg)

	producedTotal := newCounterVec(m, &prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "mq",
		Name:      "produced_total",
		Help:      "Total number of messages produced",
	}, []string{"topic", "status"})

	duration := newHistogramVec(m, &prometheus.HistogramOpts{
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
	if p == nil {
		return errors.New("producer is nil")
	}
	p.mu.RLock()
	writer := p.writer
	p.mu.RUnlock()
	if writer == nil {
		return errors.New("producer not initialized")
	}
	return p.PublishToTopic(ctx, writer.Topic, key, value)
}

// PublishJSON 将对象序列化为 JSON 并发送至指定主题.
func (p *Producer) PublishJSON(ctx context.Context, topic, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}
	return p.PublishToTopic(ctx, topic, []byte(key), data)
}

// PublishToTopic 将消息发送至指定主题.
func (p *Producer) PublishToTopic(ctx context.Context, topic string, key, value []byte) error {
	if p == nil {
		return errors.New("producer is nil")
	}
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

	p.mu.RLock()
	writer := p.writer
	dlqWriter := p.dlqWriter
	logger := p.logger
	producedTotal := p.producedTotal
	duration := p.duration

	if logger == nil {
		logger = logging.Default()
	}
	if writer == nil {
		p.mu.RUnlock()
		return errors.New("producer not initialized")
	}

	err := writer.WriteMessages(ctx, msg)
	if duration != nil {
		duration.WithLabelValues(topic).Observe(time.Since(start).Seconds())
	}

	if err != nil {
		if producedTotal != nil {
			producedTotal.WithLabelValues(topic, "failed").Inc()
		}
		tracing.SetError(ctx, err)
		logger.ErrorContext(ctx, "failed to publish message", "topic", topic, "error", err)

		if dlqWriter != nil {
			if dlqErr := dlqWriter.WriteMessages(ctx, msg); dlqErr != nil {
				logger.ErrorContext(ctx, "failed to write to dlq", "error", dlqErr)
			}
		}

		p.mu.RUnlock()
		return fmt.Errorf("failed to write messages: %w", err)
	}

	if producedTotal != nil {
		producedTotal.WithLabelValues(topic, "success").Inc()
	}

	p.mu.RUnlock()
	return nil
}

// UpdateConfig 使用最新配置刷新生产者连接。
func (p *Producer) UpdateConfig(cfg *config.KafkaConfig) error {
	if p == nil {
		return errors.New("producer is nil")
	}
	if cfg == nil {
		return errors.New("kafka config is nil")
	}

	writer, dlqWriter := buildProducerWriters(cfg)

	p.mu.Lock()
	oldWriter := p.writer
	oldDLQ := p.dlqWriter
	p.writer = writer
	p.dlqWriter = dlqWriter
	logger := p.logger
	p.mu.Unlock()

	if logger == nil {
		logger = logging.Default()
	}

	if oldDLQ != nil {
		if err := oldDLQ.Close(); err != nil {
			logger.Error("failed to close old dlq writer", "error", err)
		}
	}
	if oldWriter != nil {
		if err := oldWriter.Close(); err != nil {
			logger.Error("failed to close old kafka writer", "error", err)
		}
	}

	logger.Info("kafka producer updated", "topic", cfg.Topic, "brokers", cfg.Brokers)

	return nil
}

// Close 优雅关闭主写入器及死信队列写入器.
func (p *Producer) Close() error {
	var errs []error
	if p == nil {
		return nil
	}

	p.mu.Lock()
	dlqWriter := p.dlqWriter
	writer := p.writer
	p.dlqWriter = nil
	p.writer = nil
	logger := p.logger
	p.mu.Unlock()

	if logger == nil {
		logger = logging.Default()
	}

	if dlqWriter != nil {
		if dlqErr := dlqWriter.Close(); dlqErr != nil {
			logger.Error("failed to close dlq writer", "error", dlqErr)
			errs = append(errs, dlqErr)
		}
	}

	if writer != nil {
		if wErr := writer.Close(); wErr != nil {
			logger.Error("failed to close writer", "error", wErr)
			errs = append(errs, wErr)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close producers: %w", errors.Join(errs...))
	}

	return nil
}

// Consumer 封装了 Kafka 消息消费者.
type Consumer struct {
	mu            sync.RWMutex
	reader        *kafkago.Reader
	logger        *logging.Logger
	metrics       *metrics.Metrics
	dlqWriter     *kafkago.Writer
	opts          ConsumerOptions
	consumedTotal *prometheus.CounterVec
	consumeLag    *prometheus.HistogramVec
	retryTotal    *prometheus.CounterVec
	dlqTotal      *prometheus.CounterVec
}

// NewConsumer 初始化并返回一个功能增强的 Kafka 消费者.
func NewConsumer(cfg *config.KafkaConfig, logger *logging.Logger, m *metrics.Metrics) *Consumer {
	return NewConsumerWithOptions(cfg, logger, m, nil)
}

// NewConsumerWithOptions 创建带自定义选项的 Kafka 消费者。
func NewConsumerWithOptions(cfg *config.KafkaConfig, logger *logging.Logger, m *metrics.Metrics, opts *ConsumerOptions) *Consumer {
	if logger == nil {
		logger = logging.Default()
	}
	normalized := normalizeConsumerOptions(cfg, opts)

	dialer := buildDialer(cfg)
	r := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		MinBytes:       normalizeMinBytes(cfg.MinBytes),
		MaxBytes:       normalizeMaxBytes(cfg.MaxBytes),
		MaxWait:        normalizeMaxWait(cfg.MaxWait),
		Dialer:         dialer,
		CommitInterval: 0,
	})

	var dlqWriter *kafkago.Writer
	if normalized.DLQEnabled {
		dlqTopic := normalized.DLQTopic
		if dlqTopic == "" {
			dlqTopic = defaultDLQTopic(cfg.Topic)
		}
		transport := buildTransport(cfg)
		dlqWriter = &kafkago.Writer{
			Addr:         kafkago.TCP(cfg.Brokers...),
			Topic:        dlqTopic,
			Balancer:     &kafkago.LeastBytes{},
			Transport:    transport,
			RequiredAcks: kafkago.RequireAll,
		}
	}

	consumedTotal := newCounterVec(m, &prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "mq",
		Name:      "consumed_total",
		Help:      "Total number of messages consumed",
	}, []string{"topic", "status"})

	consumeLag := newHistogramVec(m, &prometheus.HistogramOpts{
		Namespace: "pkg",
		Subsystem: "mq",
		Name:      "consume_lag_seconds",
		Help:      "Lag of message consuming",
		Buckets:   []float64{0.1, 0.5, 1, 5, 10},
	}, []string{"topic"})

	retryTotal := newCounterVec(m, &prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "mq",
		Name:      "consume_retry_total",
		Help:      "Total number of message retries",
	}, []string{"topic"})

	dlqTotal := newCounterVec(m, &prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "mq",
		Name:      "dlq_total",
		Help:      "Total number of messages sent to DLQ",
	}, []string{"topic"})

	return &Consumer{
		reader:        r,
		logger:        logger,
		metrics:       m,
		dlqWriter:     dlqWriter,
		opts:          normalized,
		consumedTotal: consumedTotal,
		consumeLag:    consumeLag,
		retryTotal:    retryTotal,
		dlqTotal:      dlqTotal,
	}
}

// Consume 循环读取并处理消息.
func (c *Consumer) Consume(ctx context.Context, handler Handler) error {
	for {
		reader, dlqWriter, opts, logger := c.snapshot()
		if reader == nil {
			return errors.New("kafka consumer not initialized")
		}

		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("context cancelled during fetch: %w", ctx.Err())
			}

			logger.Error("failed to fetch message", "error", err)

			continue
		}
		if err := c.processMessage(ctx, reader, dlqWriter, logger, opts, msg, handler); err != nil {
			logger.ErrorContext(ctx, "message processing failed", "error", err, "topic", msg.Topic)
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, reader *kafkago.Reader, dlqWriter *kafkago.Writer, logger *logging.Logger, opts ConsumerOptions, msg kafkago.Message, handler Handler) error {
	carrier := propagation.MapCarrier{}
	for _, h := range msg.Headers {
		carrier[h.Key] = string(h.Value)
	}

	extractedCtx := otel.GetTextMapPropagator().Extract(ctx, carrier)
	spanCtx, span := tracing.Tracer().Start(extractedCtx, "Kafka.Consume", trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End()

	idempotencyKey, idempotencyEnabled := c.prepareIdempotency(spanCtx, msg, opts, logger)
	if idempotencyEnabled && idempotencyKey != "" {
		ok, _, err := opts.IdempotencyManager.TryStart(spanCtx, idempotencyKey, opts.IdempotencyTTL)
		if err != nil {
			logger.ErrorContext(spanCtx, "idempotency check failed, continue processing", "error", err, "key", idempotencyKey)
		} else if !ok {
			logger.InfoContext(spanCtx, "duplicate message skipped", "key", idempotencyKey, "topic", msg.Topic)
			return c.commitMessage(spanCtx, reader, logger, msg)
		}
	}

	retryable := opts.Retryable
	if retryable == nil {
		retryable = func(error) bool { return true }
	}

	attempt := 0
	handleErr := retry.If(spanCtx, func() error {
		attempt++
		if attempt > 1 {
			c.retryTotal.WithLabelValues(msg.Topic).Inc()
		}
		return handler(spanCtx, msg)
	}, retryable, opts.RetryConfig)

	c.consumeLag.WithLabelValues(msg.Topic).Observe(time.Since(msg.Time).Seconds())

	if handleErr != nil {
		c.consumedTotal.WithLabelValues(msg.Topic, "failed").Inc()
		tracing.SetError(spanCtx, handleErr)
		logger.ErrorContext(spanCtx, "message handler failed", "error", handleErr, "topic", msg.Topic)

		if opts.DLQEnabled {
			if err := c.publishDLQ(spanCtx, msg, handleErr, dlqWriter, logger); err != nil {
				return err
			}
			c.dlqTotal.WithLabelValues(msg.Topic).Inc()
			return c.commitMessage(spanCtx, reader, logger, msg)
		}

		if opts.CommitOnError {
			return c.commitMessage(spanCtx, reader, logger, msg)
		}

		return nil
	}

	if idempotencyEnabled && idempotencyKey != "" {
		finishErr := opts.IdempotencyManager.Finish(spanCtx, idempotencyKey, &idempotency.Response{
			Header:     map[string]string{"source": "kafka"},
			Body:       "OK",
			StatusCode: 200,
		}, opts.IdempotencyTTL)
		if finishErr != nil {
			logger.ErrorContext(spanCtx, "idempotency finish failed", "error", finishErr, "key", idempotencyKey)
		}
	}

	if err := c.commitMessage(spanCtx, reader, logger, msg); err != nil {
		return err
	}

	c.consumedTotal.WithLabelValues(msg.Topic, "success").Inc()
	return nil
}

func (c *Consumer) prepareIdempotency(ctx context.Context, msg kafkago.Message, opts ConsumerOptions, logger *logging.Logger) (string, bool) {
	if opts.IdempotencyManager == nil {
		return "", false
	}
	key := opts.IdempotencyKeyFunc(msg)
	if key == "" {
		logger.WarnContext(ctx, "idempotency key is empty, skip", "topic", msg.Topic)
		return "", false
	}
	return key, true
}

func (c *Consumer) publishDLQ(ctx context.Context, msg kafkago.Message, err error, dlqWriter *kafkago.Writer, logger *logging.Logger) error {
	if dlqWriter == nil {
		return fmt.Errorf("dlq writer is not configured")
	}

	headers := make([]kafkago.Header, 0, len(msg.Headers)+4)
	headers = append(headers, msg.Headers...)
	headers = append(headers,
		kafkago.Header{Key: headerOriginalTopic, Value: []byte(msg.Topic)},
		kafkago.Header{Key: headerOriginalPartition, Value: []byte(fmt.Sprintf("%d", msg.Partition))},
		kafkago.Header{Key: headerOriginalOffset, Value: []byte(fmt.Sprintf("%d", msg.Offset))},
		kafkago.Header{Key: headerError, Value: []byte(err.Error())},
	)

	dlqMsg := kafkago.Message{
		Topic:   dlqWriter.Topic,
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: headers,
		Time:    time.Now(),
	}

	if writeErr := dlqWriter.WriteMessages(ctx, dlqMsg); writeErr != nil {
		logger.ErrorContext(ctx, "failed to write to dlq", "error", writeErr, "topic", dlqWriter.Topic)
		return writeErr
	}

	return nil
}

func (c *Consumer) commitMessage(ctx context.Context, reader *kafkago.Reader, logger *logging.Logger, msg kafkago.Message) error {
	if reader == nil {
		return errors.New("kafka reader is nil")
	}
	if err := reader.CommitMessages(ctx, msg); err != nil {
		logger.ErrorContext(ctx, "commit failed", "error", err, "topic", msg.Topic)
		return err
	}

	return nil
}

// Start 并发启动多个消费者协程.
func (c *Consumer) Start(ctx context.Context, workers int, handler Handler) {
	for i := 0; i < workers; i++ {
		go func() {
			if err := c.Consume(ctx, handler); err != nil && !errors.Is(err, context.Canceled) {
				_, _, _, logger := c.snapshot()
				logger.Error("consumer stopped", "error", err)
			}
		}()
	}
}

// Close 关闭消费者.
func (c *Consumer) Close() error {
	if c == nil {
		return nil
	}
	reader, dlqWriter, _, logger := c.snapshot()
	if dlqWriter != nil {
		if err := dlqWriter.Close(); err != nil {
			logger.Error("failed to close dlq writer", "error", err)
		}
	}
	if reader != nil {
		if err := reader.Close(); err != nil {
			return fmt.Errorf("failed to close kafka reader: %w", err)
		}
	}

	return nil
}

// UpdateConfig 使用最新配置刷新消费者连接与治理参数。
// 注意：更新会主动关闭旧 reader，可能导致少量消息重复消费，需保证幂等。
func (c *Consumer) UpdateConfig(cfg *config.KafkaConfig, opts *ConsumerOptions) error {
	if c == nil {
		return errors.New("consumer is nil")
	}
	if cfg == nil {
		return errors.New("kafka config is nil")
	}

	logger := c.logger
	if logger == nil {
		logger = logging.Default()
	}

	normalized := normalizeConsumerOptions(cfg, opts)
	dialer := buildDialer(cfg)
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		MinBytes:       normalizeMinBytes(cfg.MinBytes),
		MaxBytes:       normalizeMaxBytes(cfg.MaxBytes),
		MaxWait:        normalizeMaxWait(cfg.MaxWait),
		Dialer:         dialer,
		CommitInterval: 0,
	})

	var dlqWriter *kafkago.Writer
	if normalized.DLQEnabled {
		dlqTopic := normalized.DLQTopic
		if dlqTopic == "" {
			dlqTopic = defaultDLQTopic(cfg.Topic)
		}
		transport := buildTransport(cfg)
		dlqWriter = &kafkago.Writer{
			Addr:         kafkago.TCP(cfg.Brokers...),
			Topic:        dlqTopic,
			Balancer:     &kafkago.LeastBytes{},
			Transport:    transport,
			RequiredAcks: kafkago.RequireAll,
		}
	}

	c.mu.Lock()
	oldReader := c.reader
	oldDLQ := c.dlqWriter
	c.reader = reader
	c.dlqWriter = dlqWriter
	c.opts = normalized
	c.mu.Unlock()

	if oldDLQ != nil {
		if err := oldDLQ.Close(); err != nil {
			logger.Error("failed to close old dlq writer", "error", err)
		}
	}
	if oldReader != nil {
		if err := oldReader.Close(); err != nil {
			logger.Error("failed to close old kafka reader", "error", err)
		}
	}

	logger.Info("kafka consumer updated", "topic", cfg.Topic, "group", cfg.GroupID, "brokers", cfg.Brokers)

	return nil
}

func (c *Consumer) snapshot() (*kafkago.Reader, *kafkago.Writer, ConsumerOptions, *logging.Logger) {
	if c == nil {
		return nil, nil, ConsumerOptions{}, logging.Default()
	}

	c.mu.RLock()
	reader := c.reader
	dlqWriter := c.dlqWriter
	opts := c.opts
	logger := c.logger
	c.mu.RUnlock()

	if logger == nil {
		logger = logging.Default()
	}

	return reader, dlqWriter, opts, logger
}

func normalizeConsumerOptions(cfg *config.KafkaConfig, opts *ConsumerOptions) ConsumerOptions {
	if opts == nil {
		return ConsumerOptions{
			RetryConfig:        buildRetryConfig(cfg),
			DLQEnabled:         cfg.DLQEnabled,
			DLQTopic:           cfg.DLQTopic,
			CommitOnError:      cfg.CommitOnError,
			IdempotencyTTL:     defaultIdempotencyTTL,
			IdempotencyKeyFunc: defaultIdempotencyKey,
		}
	}

	normalized := *opts
	if normalized.RetryConfig == (retry.Config{}) {
		normalized.RetryConfig = retry.Config{MaxRetries: 0}
	}
	if normalized.IdempotencyTTL <= 0 {
		normalized.IdempotencyTTL = defaultIdempotencyTTL
	}
	if normalized.IdempotencyKeyFunc == nil {
		normalized.IdempotencyKeyFunc = defaultIdempotencyKey
	}
	if normalized.DLQEnabled && normalized.DLQTopic == "" {
		normalized.DLQTopic = defaultDLQTopic(cfg.Topic)
	}

	return normalized
}

func buildRetryConfig(cfg *config.KafkaConfig) retry.Config {
	if cfg.RetryMax == 0 && cfg.RetryInitial == 0 && cfg.RetryMaxBackoff == 0 && cfg.RetryMultiplier == 0 && cfg.RetryJitter == 0 {
		return retry.Config{MaxRetries: 0}
	}

	base := retry.DefaultRetryConfig()
	if cfg.RetryMax != 0 {
		base.MaxRetries = cfg.RetryMax
	}
	if cfg.RetryInitial != 0 {
		base.InitialBackoff = cfg.RetryInitial
	}
	if cfg.RetryMaxBackoff != 0 {
		base.MaxBackoff = cfg.RetryMaxBackoff
	}
	if cfg.RetryMultiplier != 0 {
		base.Multiplier = cfg.RetryMultiplier
	}
	if cfg.RetryJitter != 0 {
		base.Jitter = cfg.RetryJitter
	}

	return base
}

func defaultIdempotencyKey(msg kafkago.Message) string {
	if value := headerValue(msg, headerIdempotencyKey); value != "" {
		return value
	}
	if len(msg.Key) > 0 {
		return string(msg.Key)
	}
	return fmt.Sprintf("%s:%d:%d", msg.Topic, msg.Partition, msg.Offset)
}

func headerValue(msg kafkago.Message, key string) string {
	for _, header := range msg.Headers {
		if strings.EqualFold(header.Key, key) {
			return string(header.Value)
		}
	}
	return ""
}

func defaultDLQTopic(topic string) string {
	if topic == "" {
		topic = "default"
	}
	return topic + ".dlq"
}

func normalizeMinBytes(value int) int {
	if value <= 0 {
		return defaultMinBytes
	}
	return value
}

func normalizeMaxBytes(value int) int {
	if value <= 0 {
		return defaultMaxBytes
	}
	return value
}

func normalizeMaxWait(value time.Duration) time.Duration {
	if value <= 0 {
		return defaultMaxWait
	}
	return value
}

func buildDialer(cfg *config.KafkaConfig) *kafkago.Dialer {
	if cfg.DialTimeout <= 0 {
		return nil
	}
	return &kafkago.Dialer{
		Timeout: cfg.DialTimeout,
	}
}

func buildTransport(cfg *config.KafkaConfig) kafkago.RoundTripper {
	if cfg.DialTimeout <= 0 {
		return nil
	}
	return &kafkago.Transport{
		DialTimeout: cfg.DialTimeout,
	}
}

func buildProducerWriters(cfg *config.KafkaConfig) (*kafkago.Writer, *kafkago.Writer) {
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultProducerMaxAttempts
	}
	requiredAcks := kafkago.RequireAll
	if cfg.RequiredAcks != 0 {
		requiredAcks = kafkago.RequiredAcks(cfg.RequiredAcks)
	}

	transport := buildTransport(cfg)
	writer := &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafkago.Hash{Hasher: nil},
		Transport:    transport,
		WriteTimeout: cfg.WriteTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		MaxAttempts:  maxAttempts,
		RequiredAcks: requiredAcks,
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
		Transport:    transport,
		RequiredAcks: kafkago.RequireOne,
	}

	return writer, dlqWriter
}

func newCounterVec(m *metrics.Metrics, opts *prometheus.CounterOpts, labels []string) *prometheus.CounterVec {
	if m == nil {
		return prometheus.NewCounterVec(*opts, labels)
	}
	return m.NewCounterVec(opts, labels)
}

func newHistogramVec(m *metrics.Metrics, opts *prometheus.HistogramOpts, labels []string) *prometheus.HistogramVec {
	if m == nil {
		return prometheus.NewHistogramVec(*opts, labels)
	}
	return m.NewHistogramVec(opts, labels)
}
