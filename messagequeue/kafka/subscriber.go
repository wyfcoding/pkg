package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/eventsourcing"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/messagequeue"
	"github.com/wyfcoding/pkg/metrics"
)

var (
	// ErrTopicEmpty 订阅主题为空。
	ErrTopicEmpty = errors.New("topic is empty")
	// ErrAlreadySubscribed 主题重复订阅。
	ErrAlreadySubscribed = errors.New("topic already subscribed")
	// ErrNotSubscribed 主题未订阅。
	ErrNotSubscribed = errors.New("topic not subscribed")
)

// EventDecoder 定义事件解码函数。
type EventDecoder func(data []byte) (eventsourcing.DomainEvent, error)

// EventSubscriber 基于 Kafka 的事件订阅器实现。
type EventSubscriber struct {
	baseCfg      config.KafkaConfig
	logger       *logging.Logger
	metrics      *metrics.Metrics
	consumerOpts *ConsumerOptions
	workers      int
	decoder      EventDecoder

	mu        sync.Mutex
	consumers map[string]*Consumer
	cancels   map[string]context.CancelFunc
}

// NewEventSubscriber 创建 Kafka 事件订阅器。
func NewEventSubscriber(cfg *config.KafkaConfig, logger *logging.Logger, m *metrics.Metrics, opts *ConsumerOptions) *EventSubscriber {
	base := config.KafkaConfig{}
	if cfg != nil {
		base = *cfg
	}
	if logger == nil {
		logger = logging.Default()
	}

	return &EventSubscriber{
		baseCfg:      base,
		logger:       logger,
		metrics:      m,
		consumerOpts: opts,
		workers:      1,
		decoder:      defaultEventDecoder,
		consumers:    make(map[string]*Consumer),
		cancels:      make(map[string]context.CancelFunc),
	}
}

// WithWorkers 设置订阅消费者并发数。
func (s *EventSubscriber) WithWorkers(workers int) *EventSubscriber {
	if workers > 0 {
		s.workers = workers
	}
	return s
}

// WithDecoder 设置事件解码器。
func (s *EventSubscriber) WithDecoder(decoder EventDecoder) *EventSubscriber {
	if decoder != nil {
		s.decoder = decoder
	}
	return s
}

// Subscribe 订阅指定主题的事件流。
func (s *EventSubscriber) Subscribe(ctx context.Context, topic string, handler messagequeue.EventHandler) error {
	if topic == "" {
		return ErrTopicEmpty
	}
	if handler == nil {
		return errors.New("handler is nil")
	}

	s.mu.Lock()
	if _, ok := s.consumers[topic]; ok {
		s.mu.Unlock()
		return ErrAlreadySubscribed
	}

	cfg := s.baseCfg
	cfg.Topic = topic
	consumer := NewConsumerWithOptions(&cfg, s.logger, s.metrics, s.consumerOpts)
	consumeCtx, cancel := context.WithCancel(ctx)
	s.consumers[topic] = consumer
	s.cancels[topic] = cancel
	s.mu.Unlock()

	consumer.Start(consumeCtx, s.workers, func(msgCtx context.Context, msg kafkago.Message) error {
		event, decodeErr := s.decoder(msg.Value)
		if decodeErr != nil {
			return fmt.Errorf("decode event failed: %w", decodeErr)
		}
		return handler(msgCtx, event)
	})

	s.logger.Info("kafka event subscriber started", "topic", topic, "workers", s.workers)
	return nil
}

// Unsubscribe 取消订阅并关闭消费者。
func (s *EventSubscriber) Unsubscribe(_ context.Context, topic string) error {
	s.mu.Lock()
	consumer, ok := s.consumers[topic]
	cancel := s.cancels[topic]
	delete(s.consumers, topic)
	delete(s.cancels, topic)
	s.mu.Unlock()

	if !ok {
		return ErrNotSubscribed
	}

	if cancel != nil {
		cancel()
	}
	if consumer != nil {
		return consumer.Close()
	}
	return nil
}

// Close 关闭所有订阅消费者。
func (s *EventSubscriber) Close() error {
	s.mu.Lock()
	topics := make([]string, 0, len(s.consumers))
	for topic := range s.consumers {
		topics = append(topics, topic)
	}
	s.mu.Unlock()

	var errs []error
	for _, topic := range topics {
		if err := s.Unsubscribe(context.Background(), topic); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func defaultEventDecoder(data []byte) (eventsourcing.DomainEvent, error) {
	var event eventsourcing.BaseEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}
