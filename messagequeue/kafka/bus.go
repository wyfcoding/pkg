// Package kafka 提供了基于 Kafka 的事件总线与生产者实现。
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/wyfcoding/pkg/eventsourcing"
)

// TopicMapper 定义了根据领域事件类型动态决定 Kafka 主题 (Topic) 的路由逻辑。
type TopicMapper func(event eventsourcing.DomainEvent) string

// EventBus 实现了 messagequeue.EventBus 接口，利用 Kafka 提供可靠的异步事件分发能力。
type EventBus struct {
	producer    *Producer   // 底层封装的 Kafka 生产者。
	topicMapper TopicMapper // 事件路由映射器。
}

// NewEventBus 创建 EventBus 实例。
// producer: 底层 Kafka 生产者。
// defaultTopic: 默认主题，如果映射失败或未提供映射器时使用。
func NewEventBus(producer *Producer, defaultTopic string) *EventBus {
	bus := &EventBus{
		producer: producer,
	}

	// 默认映射逻辑：使用指定主题。
	bus.topicMapper = func(_ eventsourcing.DomainEvent) string {
		return defaultTopic
	}

	return bus
}

// WithTopicMapper 设置主题映射策略。
func (b *EventBus) WithTopicMapper(mapper TopicMapper) *EventBus {
	b.topicMapper = mapper

	return b
}

// Publish 执行单个领域事件的异步分发。
func (b *EventBus) Publish(ctx context.Context, event eventsourcing.DomainEvent) error {
	topic := b.topicMapper(event)
	key := []byte(event.AggregateID())

	// 将领域事件对象序列化为标准 JSON 格式。
	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal domain event: %w", err)
	}

	// 调用生产者执行带链路追踪和指标采集的物理发送。
	if sendErr := b.producer.PublishToTopic(ctx, topic, key, value); sendErr != nil {
		return sendErr
	}

	slog.InfoContext(ctx, "domain event published to bus",
		"topic", topic,
		"aggregate_id", event.AggregateID(),
		"event_type", event.EventType(),
	)

	return nil
}

// PublishBatch 批量发布领域事件。
func (b *EventBus) PublishBatch(ctx context.Context, events []eventsourcing.DomainEvent) error {
	for _, event := range events {
		if err := b.Publish(ctx, event); err != nil {
			return err
		}
	}

	return nil
}

// Close 关闭事件总线。
func (b *EventBus) Close() error {
	return b.producer.Close()
}
