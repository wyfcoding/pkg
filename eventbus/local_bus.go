// Package eventbus 提供了进程内的发布/订阅通信模型。
// 增强：实现了一个基于内存的高性能事件总线，用于本地模块解耦。
package eventbus

import (
	"context"
	"sync"

	"github.com/wyfcoding/pkg/eventsourcing"
	"github.com/wyfcoding/pkg/messagequeue"
)

// LocalBus 基于内存的本地事件总线实现。
type LocalBus struct {
	mu          sync.RWMutex
	subscribers map[string][]messagequeue.EventHandler
}

// NewLocalBus 创建一个新的本地事件总线。
func NewLocalBus() *LocalBus {
	return &LocalBus{
		subscribers: make(map[string][]messagequeue.EventHandler),
	}
}

// Publish 发布事件到总线。
func (b *LocalBus) Publish(ctx context.Context, event eventsourcing.DomainEvent) error {
	b.mu.RLock()
	handlers, ok := b.subscribers[event.EventType()]
	b.mu.RUnlock()

	if !ok {
		return nil
	}

	for _, handler := range handlers {
		// 这里选择异步执行以保证发布者的性能，根据实际需求也可改为同步
		go func(h messagequeue.EventHandler, e eventsourcing.DomainEvent) {
			_ = h(ctx, e)
		}(handler, event)
	}

	return nil
}

// Subscribe 订阅特定主题的事件。
func (b *LocalBus) Subscribe(ctx context.Context, topic string, handler messagequeue.EventHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.subscribers[topic] = append(b.subscribers[topic], handler)
	return nil
}

// Unsubscribe 取消订阅主题（此处为简化版实现，实际可增加特定 handler 移除逻辑）。
func (b *LocalBus) Unsubscribe(ctx context.Context, topic string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.subscribers, topic)
	return nil
}
