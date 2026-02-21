// 变更说明：定义跨微服务的标准化事件总线。
// 支持 Kafka 实现，具备幂等生产、重试和消费者组管理能力。
package eventbus

import (
	"context"
)

type EventHandler func(ctx context.Context, event []byte) error

type EventBus interface {
	// Publish 发布事件到远端 Topic
	Publish(ctx context.Context, topic string, payload interface{}) error
	// Subscribe 订阅 Topic 并绑定处理器
	Subscribe(ctx context.Context, topic string, handler EventHandler) error
	// Close 关闭总线连接
	Close() error
}
