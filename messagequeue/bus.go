package messagequeue

import (
	"context"

	"github.com/wyfcoding/pkg/eventsourcing"
)

// EventBus 事件总线接口
// 用于在服务间或模块间异步发布领域事件
type EventBus interface {
	// Publish 发布单个领域事件
	Publish(ctx context.Context, event eventsourcing.DomainEvent) error
	// PublishBatch 批量发布领域事件
	PublishBatch(ctx context.Context, events []eventsourcing.DomainEvent) error
	// Close 关闭总线连接
	Close() error
}

// EventHandler 事件处理函数
type EventHandler func(ctx context.Context, event eventsourcing.DomainEvent) error

// EventSubscriber 事件订阅者接口
type EventSubscriber interface {
	// Subscribe 订阅特定类型的事件
	Subscribe(ctx context.Context, topic string, handler EventHandler) error
	// Unsubscribe 取消订阅
	Unsubscribe(ctx context.Context, topic string) error
}
