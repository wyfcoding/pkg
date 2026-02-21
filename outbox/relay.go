// Package outbox 消息推送逻辑
package outbox

import (
	"context"
	"time"

	"github.com/wyfcoding/pkg/messagequeue"
)

type Relay struct {
	manager   *Manager
	publisher messagequeue.EventPublisher
}

func (r *Relay) Start(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processPendingMessages(ctx)
		}
	}
}

func (r *Relay) processPendingMessages(ctx context.Context) {
	var messages []OutboxMessage
	r.manager.db.Where("status = ?", "PENDING").Limit(100).Find(&messages)

	for _, msg := range messages {
		err := r.publisher.Publish(ctx, msg.Topic, msg.ID, msg.Payload)
		if err == nil {
			now := time.Now()
			r.manager.db.Model(&msg).Updates(map[string]interface{}{
				"status":  "SENT",
				"sent_at": &now,
			})
		} else {
			r.manager.db.Model(&msg).Update("retries", msg.Retries+1)
		}
	}
}
