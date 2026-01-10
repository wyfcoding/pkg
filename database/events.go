package database

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// DomainEvent 领域事件接口.
type DomainEvent interface {
	EventName() string
	EventKey() string
}

// EntityEvents 定义了实体携带事件的能力.
type EntityEvents interface {
	GetEvents() []DomainEvent
	ClearEvents()
}

// BaseEntity 是一个可嵌入的结构体.
type BaseEntity struct { //nolint:govet
	events []DomainEvent `gorm:"-" json:"-"`
}

// AddEvent 添加事件.
func (e *BaseEntity) AddEvent(event DomainEvent) {
	e.events = append(e.events, event)
}

// GetEvents 获取事件.
func (e *BaseEntity) GetEvents() []DomainEvent {
	return e.events
}

// ClearEvents 清空事件.
func (e *BaseEntity) ClearEvents() {
	e.events = nil
}

// OutboxRecord 必须与业务在同一事务中持久化的记录.
type OutboxRecord struct { //nolint:govet
	CreatedAt time.Time `gorm:"index"`
	Topic     string    `gorm:"type:varchar(255);not null;index"`
	Key       string    `gorm:"type:varchar(255);index"`
	Payload   []byte    `gorm:"type:blob;not null"`
	ID        uint64    `gorm:"primarykey"`
	Status    int8      `gorm:"type:tinyint;default:0;index"` // 0: Pending, 1: Sent, 2: Failed.
}

// TableName 返回表名.
func (OutboxRecord) TableName() string {
	return "sys_outbox_messages"
}

// EventPlugin GORM 插件.
type EventPlugin struct{}

// Name 插件名称.
func (p *EventPlugin) Name() string {
	return "domain_event_plugin"
}

// Initialize 初始化插件.
func (p *EventPlugin) Initialize(db *gorm.DB) error {
	if err := db.Callback().Create().After("gorm:create").Register("event_plugin:after_create", p.handleEvents); err != nil {
		return fmt.Errorf("failed to register event plugin: %w", err)
	}

	return nil
}

func (p *EventPlugin) handleEvents(db *gorm.DB) {
	if db.Error != nil || db.Statement.Schema == nil {
		return
	}

	entity, ok := db.Statement.Dest.(EntityEvents)
	if !ok {
		return
	}

	events := entity.GetEvents()
	if len(events) == 0 {
		return
	}

	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			_ = db.AddError(fmt.Errorf("failed to marshal domain event: %w", err))

			continue
		}

		record := &OutboxRecord{
			Topic:     event.EventName(),
			Key:       event.EventKey(),
			Payload:   payload,
			Status:    0,
			CreatedAt: time.Now(),
			ID:        0,
		}

		if err := db.Session(&gorm.Session{NewDB: true}).Create(record).Error; err != nil {
			_ = db.AddError(fmt.Errorf("failed to create outbox record: %w", err))
		}
	}

	entity.ClearEvents()
}
