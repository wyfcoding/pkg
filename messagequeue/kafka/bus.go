package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wyfcoding/pkg/eventsourcing"
)

// TopicMapper 根据事件决定 Topic 的函数
type TopicMapper func(event eventsourcing.DomainEvent) string

// KafkaEventBus 基于 Kafka 的事件总线实现
type KafkaEventBus struct {
	producer    *Producer
	topicMapper TopicMapper
}

// NewKafkaEventBus 创建 KafkaEventBus
// producer: 底层 Kafka 生产者
// defaultTopic: 默认 Topic，如果 mapper 返回空或未提供 mapper 时使用
func NewKafkaEventBus(producer *Producer, defaultTopic string) *KafkaEventBus {
	bus := &KafkaEventBus{
		producer: producer,
	}

	// 默认 Mapper：使用指定topic
	bus.topicMapper = func(event eventsourcing.DomainEvent) string {
		return defaultTopic
	}

	return bus
}

// WithTopicMapper 设置 Topic 映射策略
func (b *KafkaEventBus) WithTopicMapper(mapper TopicMapper) *KafkaEventBus {
	b.topicMapper = mapper
	return b
}

// Publish 实现 EventBus 接口
func (b *KafkaEventBus) Publish(ctx context.Context, event eventsourcing.DomainEvent) error {
	topic := b.topicMapper(event)
	// 如果 mapper 返回空，可能 fallback 到 producer 默认 topic，或报错
	// 这里依赖 producer.PublishToTopic，如果没有 topic 会报错

	key := []byte(event.AggregateID())

	// 序列化事件
	// 这里假设 domain event 可以被 json 序列化
	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal domain event: %w", err)
	}

	// 实际发送
	// 注意：Producer 会自动处理 Tracing 注入
	return b.producer.PublishToTopic(ctx, topic, key, value)
}

// PublishBatch 实现 EventBus 接口
func (b *KafkaEventBus) PublishBatch(ctx context.Context, events []eventsourcing.DomainEvent) error {
	for _, event := range events {
		if err := b.Publish(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// Close 关闭总线
func (b *KafkaEventBus) Close() error {
	return b.producer.Close()
}
