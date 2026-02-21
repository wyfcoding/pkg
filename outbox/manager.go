// Package outbox 实现事务性发件箱模式
package outbox

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// OutboxMessage 待发送消息实体
type OutboxMessage struct {
	ID        string    `gorm:"primaryKey"`
	Topic     string    `gorm:"index"`
	Payload   []byte    `gorm:"type:json"`
	Status    string    `gorm:"index;default:PENDING"` // PENDING, SENT, FAILED
	Retries   int       `gorm:"default:0"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	SentAt    *time.Time
}

// Manager Outbox 管理器
type Manager struct {
	db *gorm.DB
}

func NewManager(db *gorm.DB) *Manager {
	return &Manager{db: db}
}

// WriteInTx 在现有事务中写入外发消息
func (m *Manager) WriteInTx(tx *gorm.DB, topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	msg := &OutboxMessage{
		ID:      generateID(),
		Topic:   topic,
		Payload: data,
		Status:  "PENDING",
	}
	return tx.Create(msg).Error
}

func generateID() string {
	return time.Now().Format("20060102150405-") + time.Now().Format("0000000000") // 简易 ID
}
