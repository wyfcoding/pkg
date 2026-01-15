// Package messagequeue 提供了统一的事件总线与订阅者接口定义。
package messagequeue

import (
	"context"

	"github.com/wyfcoding/pkg/eventsourcing"
)

var (
	defaultEventBus        EventBus
	defaultEventSubscriber EventSubscriber
)

// DefaultBus 返回全局默认事件总线实例。
func DefaultBus() EventBus {
	return defaultEventBus
}

// SetDefaultBus 设置全局默认事件总线实例。
func SetDefaultBus(eventBus EventBus) {
	defaultEventBus = eventBus
}

// DefaultSubscriber 返回全局默认事件订阅者实例。
func DefaultSubscriber() EventSubscriber {
	return defaultEventSubscriber
}

// SetDefaultSubscriber 设置全局默认事件订阅者实例。
func SetDefaultSubscriber(subscriber EventSubscriber) {
	defaultEventSubscriber = subscriber
}

// EventBus 事件总线接口。
// 用于在服务间或模块间异步发布领域事件。
type EventBus interface {
	// Publish 发布单个领域事件。
	Publish(ctx context.Context, event eventsourcing.DomainEvent) error
	// PublishBatch 批量发布领域事件。
	PublishBatch(ctx context.Context, events []eventsourcing.DomainEvent) error
	// Close 关闭总线连接。
	Close() error
}

// EventHandler 事件处理函数类型。
type EventHandler func(ctx context.Context, event eventsourcing.DomainEvent) error

// TypedHandler 定义了强类型的事件处理函数原型。
type TypedHandler[T any] func(ctx context.Context, event T) error

// SubscribeTyped 是一个泛型辅助函数，提供类型安全的事件订阅能力。
func SubscribeTyped[T any](ctx context.Context, sub EventSubscriber, topic string, handler TypedHandler[T]) error {
	return sub.Subscribe(ctx, topic, func(ctx context.Context, ev eventsourcing.DomainEvent) error {
		// 尝试从 BaseEvent.Data 中提取数据（如果 ev 是 *BaseEvent）
		if base, ok := ev.(*eventsourcing.BaseEvent); ok {
			if data, ok := base.Data.(T); ok {
				return handler(ctx, data)
			}
		}

		// 尝试直接转换事件对象本身
		if data, ok := ev.(T); ok {
			return handler(ctx, data)
		}

		return nil // 类型不匹配则忽略或记录警告
	})
}

// EventSubscriber 事件订阅者接口。
type EventSubscriber interface {
	// Subscribe 订阅特定类型的事件。
	Subscribe(ctx context.Context, topic string, handler EventHandler) error
	// Unsubscribe 取消订阅。
	Unsubscribe(ctx context.Context, topic string) error
}
