// Package eventsourcing 提供事件溯源的核心抽象和基础设施
// 支持领域事件、聚合根、事件存储等核心概念
package eventsourcing

import (
	"time"

	"github.com/google/uuid"
)

// DomainEvent 领域事件基础接口
// 所有领域事件必须实现此接口，用于事件溯源和事件驱动架构
type DomainEvent interface {
	// EventType 返回事件类型标识
	EventType() string
	// OccurredAt 返回事件发生时间
	OccurredAt() time.Time
	// AggregateID 返回聚合根ID
	AggregateID() string
	// Version 返回事件版本号
	Version() int64
	// SetVersion 设置事件版本号
	SetVersion(version int64)
}

// BaseEvent 领域事件基础实现
// 提供通用字段和方法，具体领域事件可嵌入此结构体
type BaseEvent struct {
	ID        string    `json:"id"`           // 事件唯一标识
	Type      string    `json:"type"`         // 事件类型
	AggID     string    `json:"aggregate_id"` // 聚合根ID
	Ver       int64     `json:"version"`      // 事件版本（聚合根版本）
	Timestamp time.Time `json:"timestamp"`    // 事件发生时间
	Metadata  Metadata  `json:"metadata"`     // 事件元数据
	Data      any       `json:"data"`         // 事件载荷
}

// Metadata 事件元数据
type Metadata struct {
	CorrelationID string            `json:"correlation_id,omitempty"` // 关联ID（用于追踪）
	CausationID   string            `json:"causation_id,omitempty"`   // 因果ID（触发此事件的事件ID）
	UserID        string            `json:"user_id,omitempty"`        // 操作用户ID
	TraceID       string            `json:"trace_id,omitempty"`       // 链路追踪ID
	Extra         map[string]string `json:"extra,omitempty"`          // 扩展元数据
}

// EventType 实现 DomainEvent 接口
func (e *BaseEvent) EventType() string {
	return e.Type
}

// OccurredAt 实现 DomainEvent 接口
func (e *BaseEvent) OccurredAt() time.Time {
	return e.Timestamp
}

// AggregateID 实现 DomainEvent 接口
func (e *BaseEvent) AggregateID() string {
	return e.AggID
}

// Version 实现 DomainEvent 接口
func (e *BaseEvent) Version() int64 {
	return e.Ver
}

// SetVersion 实现 DomainEvent 接口
func (e *BaseEvent) SetVersion(version int64) {
	e.Ver = version
}

// NewBaseEvent 创建基础事件实例
// eventType: 事件类型标识
// aggregateID: 聚合根ID
// version: 事件版本号
func NewBaseEvent(eventType, aggregateID string, version int64) BaseEvent {
	return BaseEvent{
		ID:        uuid.New().String(),
		Type:      eventType,
		AggID:     aggregateID,
		Ver:       version,
		Timestamp: time.Now(),
	}
}

// NewBaseEventWithMetadata 创建带元数据的基础事件
func NewBaseEventWithMetadata(eventType, aggregateID string, version int64, metadata Metadata) BaseEvent {
	event := NewBaseEvent(eventType, aggregateID, version)
	event.Metadata = metadata
	return event
}

// AggregateRoot 事件溯源聚合根基类
// 所有需要事件溯源的聚合根应嵌入此结构体
type AggregateRoot struct {
	id          string        // 聚合根唯一标识
	version     int64         // 当前版本号
	uncommitted []DomainEvent // 未提交的领域事件
}

// ID 返回聚合根唯一标识
func (a *AggregateRoot) ID() string {
	return a.id
}

// SetID 设置聚合根唯一标识
func (a *AggregateRoot) SetID(id string) {
	a.id = id
}

// Version 返回聚合根当前版本号
func (a *AggregateRoot) Version() int64 {
	return a.version
}

// SetVersion 设置聚合根版本号（用于从事件流恢复时）
func (a *AggregateRoot) SetVersion(version int64) {
	a.version = version
}

// ApplyChange 应用一个新的领域事件
// 将事件添加到未提交事件列表，并递增版本号
// 注意：调用此方法后还需要调用具体聚合根的Apply方法来更新状态
func (a *AggregateRoot) ApplyChange(event DomainEvent) {
	a.version++
	event.SetVersion(a.version)
	a.uncommitted = append(a.uncommitted, event)
}

// GetUncommittedEvents 获取所有未提交的领域事件
func (a *AggregateRoot) GetUncommittedEvents() []DomainEvent {
	return a.uncommitted
}

// MarkCommitted 标记所有事件已提交
// 在事件成功持久化后调用此方法清空未提交事件列表
func (a *AggregateRoot) MarkCommitted() {
	a.uncommitted = nil
}

// HasUncommittedEvents 检查是否有未提交的事件
func (a *AggregateRoot) HasUncommittedEvents() bool {
	return len(a.uncommitted) > 0
}

// EventApplier 事件应用器接口
// 聚合根需要实现此接口以支持事件重放
type EventApplier interface {
	// Apply 将事件应用到聚合根状态
	Apply(event DomainEvent)
}

// Aggregate 完整聚合接口
// 组合了聚合根基础功能和事件应用能力
type Aggregate interface {
	EventApplier
	ID() string
	Version() int64
	SetID(id string)
	SetVersion(version int64)
	ApplyChange(event DomainEvent)
	GetUncommittedEvents() []DomainEvent
	MarkCommitted()
}

// LoadFromHistory 从事件历史恢复聚合根状态
// aggregate: 需要恢复的聚合根实例
// events: 事件历史列表
func LoadFromHistory(aggregate EventApplier, events []DomainEvent) {
	for _, event := range events {
		aggregate.Apply(event)
	}
}
