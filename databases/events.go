package databases

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// DomainEvent 领域事件接口。
// 任何希望作为事件发布的结构体都应实现此接口。
type DomainEvent interface {
	EventName() string
	EventKey() string
}

// EntityEvents 定义了实体携带事件的能力。
type EntityEvents interface {
	GetEvents() []DomainEvent
	ClearEvents()
}

// BaseEntity 是一个可嵌入的结构体，为模型提供事件管理能力。
type BaseEntity struct {
	events []DomainEvent `gorm:"-" json:"-"`
}

func (e *BaseEntity) AddEvent(event DomainEvent) {
	e.events = append(e.events, event)
}

func (e *BaseEntity) GetEvents() []DomainEvent {
	return e.events
}

func (e *BaseEntity) ClearEvents() {
	e.events = nil
}

// OutboxRecord 必须与业务在同一事务中持久化的记录。
type OutboxRecord struct {
	ID        uint64    `gorm:"primarykey"`
	Topic     string    `gorm:"type:varchar(255);not null;index"`
	Key       string    `gorm:"type:varchar(255);index"`
	Payload   []byte    `gorm:"type:blob;not null"`
	Status    int8      `gorm:"type:tinyint;default:0;index"` // 0: Pending, 1: Sent, 2: Failed
	CreatedAt time.Time `gorm:"index"`
}

func (OutboxRecord) TableName() string {
	return "sys_outbox_messages"
}

// EventPlugin GORM 插件，用于自动处理实体中的事件。
type EventPlugin struct{}

func (p *EventPlugin) Name() string {
	return "domain_event_plugin"
}

func (p *EventPlugin) Initialize(db *gorm.DB) error {
	// 注册在创建/更新之后的钩子
	return db.Callback().Create().After("gorm:create").Register("event_plugin:after_create", p.handleEvents)
}

func (p *EventPlugin) handleEvents(db *gorm.DB) {
	if db.Error != nil || db.Statement.Schema == nil {
		return
	}

	// 检查模型是否实现了 EntityEvents 接口
	entity, ok := db.Statement.Dest.(EntityEvents)
	if !ok {
		return
	}

	events := entity.GetEvents()
	if len(events) == 0 {
		return
	}

	for _, event := range events {
		payload, _ := json.Marshal(event)
		record := &OutboxRecord{
			Topic:     event.EventName(),
			Key:       event.EventKey(),
			Payload:   payload,
			Status:    0,
			CreatedAt: time.Now(),
		}
		// 在当前事务中保存到 outbox 表
		if err := db.Session(&gorm.Session{NewDB: true}).Table("sys_outbox_messages").Create(record).Error; err != nil {
			db.AddError(err)
		}
	}

	entity.ClearEvents()
}
