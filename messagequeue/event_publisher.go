package messagequeue

import "context"

// EventPublisher 定义了领域事件发布的通用接口。
// 它解耦了领域层与具体的消息队列实现（如 Kafka, RabbitMQ）以及发布模式（如 Outbox）。
type EventPublisher interface {
	// Publish 发布一个普通事件，通常用于非事务性或最终一致性要求较低的情况。
	Publish(ctx context.Context, topic string, key string, event any) error

	// PublishInTx 在事务中发布事件，核心用于保证业务操作与事件发布的原子性（Outbox 模式）。
	// tx 参数通常是底层数据库的事务对象，如 *gorm.DB。
	PublishInTx(ctx context.Context, tx any, topic string, key string, event any) error
}
