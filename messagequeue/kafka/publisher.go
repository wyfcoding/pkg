// Package kafka 提供了基于 Kafka 的 EventPublisher 实现。
package kafka

import (
	"context"

	"github.com/wyfcoding/pkg/messagequeue"
)

// EventPublisher 实现了 messagequeue.EventPublisher 接口。
type EventPublisher struct {
	producer *Producer
}

// NewEventPublisher 创建一个新的 Kafka 事件发布器。
func NewEventPublisher(producer *Producer) messagequeue.EventPublisher {
	return &EventPublisher{
		producer: producer,
	}
}

// Publish 发布消息。
func (p *EventPublisher) Publish(ctx context.Context, topic string, key string, event any) error {
	return p.producer.PublishJSON(ctx, topic, key, event)
}

// PublishInTx 在事务内发布消息（Kafka 实现暂不支持原子性事务，回退到非事务发布）。
// 如果需要强一致性事务，请使用 outbox 模式。
func (p *EventPublisher) PublishInTx(ctx context.Context, tx any, topic string, key string, event any) error {
	// TODO: 如果底层使用可支持事务的 Kafka 客户端，可在此实现。
	return p.Publish(ctx, topic, key, event)
}
