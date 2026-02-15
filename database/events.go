package database

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/wyfcoding/pkg/messagequeue/outbox"
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
type BaseEntity struct {
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

// EventPlugin GORM 插件，自动将实体中的领域事件转换为 Outbox 消息。
type EventPlugin struct{}

// Name 插件名称.
func (p *EventPlugin) Name() string {
	return "domain_event_plugin"
}

// Initialize 初始化插件.
func (p *EventPlugin) Initialize(db *gorm.DB) error {
	// 注册在 Create/Update 之后的回调
	if err := db.Callback().Create().After("gorm:create").Register("event_plugin:after_create", p.handleEvents); err != nil {
		return fmt.Errorf("failed to register event plugin on create: %w", err)
	}
	if err := db.Callback().Update().After("gorm:update").Register("event_plugin:after_update", p.handleEvents); err != nil {
		return fmt.Errorf("failed to register event plugin on update: %w", err)
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

		// 使用通用的 outbox.Message 模型
		record := &outbox.Message{
			Topic:     event.EventName(),
			Key:       event.EventKey(),
			Payload:   payload,
			Status:    outbox.StatusPending,
			NextRetry: time.Now(),
		}

		// 在同一事务中保存 Outbox 记录
		if err := db.Session(&gorm.Session{NewDB: true}).Table(record.TableName()).Create(record).Error; err != nil {
			_ = db.AddError(fmt.Errorf("failed to create outbox record: %w", err))
		}
	}

	entity.ClearEvents()
}
