// 生成摘要：新增通用 Outbox 发布器，供各服务复用。
// 假设：调用方优先使用 PublishInTx 保证事务一致性。
package outbox

import (
	"context"

	"gorm.io/gorm"
)

// Publisher 通用 Outbox 事件发布器（带 key）。
type Publisher struct {
	mgr *Manager
}

// NewPublisher 创建通用 Outbox 发布器（带 key）。
func NewPublisher(mgr *Manager) *Publisher {
	return &Publisher{mgr: mgr}
}

// Publish 在非事务环境发布事件（仅作兜底，不保证强一致）。
func (p *Publisher) Publish(ctx context.Context, topic string, key string, event any) error {
	return p.mgr.PublishInTx(ctx, p.mgr.DB(), topic, key, event)
}

// PublishInTx 在事务内发布事件（推荐使用）。
func (p *Publisher) PublishInTx(ctx context.Context, tx any, topic string, key string, event any) error {
	gormTx, ok := tx.(*gorm.DB)
	if !ok {
		return p.Publish(ctx, topic, key, event)
	}
	return p.mgr.PublishInTx(ctx, gormTx, topic, key, event)
}
