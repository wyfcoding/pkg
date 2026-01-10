// Package eventsourcing 提供事件溯源的核心抽象和基础设施。
package eventsourcing

import (
	"time"

	"github.com/google/uuid"
)

// DomainEvent 领域事件基础接口。
type DomainEvent interface {
	// EventType 返回事件类型标识。
	EventType() string
	// OccurredAt 返回事件发生时间。
	OccurredAt() time.Time
	// AggregateID 返回聚合根 ID。
	AggregateID() string
	// Version 返回事件版本号。
	Version() int64
	// SetVersion 设置事件版本号。
	SetVersion(version int64)
}

// BaseEvent 领域事件基础实现。
type BaseEvent struct {
	Timestamp time.Time `json:"timestamp"`    // 事件发生时间。
	Metadata  Metadata  `json:"metadata"`     // 事件元数据。
	Data      any       `json:"data"`         // 事件载荷。
	ID        string    `json:"id"`           // 事件唯一标识。
	Type      string    `json:"type"`         // 事件类型。
	AggID     string    `json:"aggregate_id"` // 聚合根 ID。
	Ver       int64     `json:"version"`      // 事件版本（聚合根版本）。
}

// Metadata 事件元数据。
type Metadata struct {
	Extra         map[string]string `json:"extra,omitempty"`          // 扩展元数据。
	CorrelationID string            `json:"correlation_id,omitempty"` // 关联 ID（用于追踪）。
	CausationID   string            `json:"causation_id,omitempty"`   // 因果 ID。
	UserID        string            `json:"user_id,omitempty"`        // 操作用户 ID。
	TraceID       string            `json:"trace_id,omitempty"`       // 链路追踪 ID。
}

// EventType 实现 DomainEvent 接口。
func (e *BaseEvent) EventType() string {
	return e.Type
}

// OccurredAt 实现 DomainEvent 接口。
func (e *BaseEvent) OccurredAt() time.Time {
	return e.Timestamp
}

// AggregateID 实现 DomainEvent 接口。
func (e *BaseEvent) AggregateID() string {
	return e.AggID
}

// Version 实现 DomainEvent 接口。
func (e *BaseEvent) Version() int64 {
	return e.Ver
}

// SetVersion 实现 DomainEvent 接口。
func (e *BaseEvent) SetVersion(version int64) {
	e.Ver = version
}

// NewBaseEvent 创建基础事件实例。
func NewBaseEvent(eventType, aggregateID string, version int64) BaseEvent {
	return BaseEvent{
		ID:        uuid.New().String(),
		Type:      eventType,
		AggID:     aggregateID,
		Ver:       version,
		Timestamp: time.Now(),
		Metadata: Metadata{
			CorrelationID: "",
			CausationID:   "",
			UserID:        "",
			TraceID:       "",
			Extra:         make(map[string]string),
		},
		Data: nil,
	}
}

// NewBaseEventWithMetadata 创建带元数据的基础事件。
func NewBaseEventWithMetadata(eventType, aggregateID string, version int64, metadata Metadata) BaseEvent {
	event := NewBaseEvent(eventType, aggregateID, version)
	event.Metadata = metadata

	return event
}

// AggregateRoot 事件溯源聚合根基类。
type AggregateRoot struct {
	uncommitted []DomainEvent
	version     int64
	id          string
}

// ID 返回聚合根唯一标识。
func (a *AggregateRoot) ID() string {
	return a.id
}

// SetID 设置聚合根唯一标识。
func (a *AggregateRoot) SetID(id string) {
	a.id = id
}

// Version 返回聚合根当前版本号。
func (a *AggregateRoot) Version() int64 {
	return a.version
}

// SetVersion 设置聚合根版本号。
func (a *AggregateRoot) SetVersion(version int64) {
	a.version = version
}

// ApplyChange 应用一个新的领域事件。
func (a *AggregateRoot) ApplyChange(event DomainEvent) {
	a.version++
	event.SetVersion(a.version)
	a.uncommitted = append(a.uncommitted, event)
}

// GetUncommittedEvents 获取所有未提交的领域事件。
func (a *AggregateRoot) GetUncommittedEvents() []DomainEvent {
	return a.uncommitted
}

// MarkCommitted 标记所有事件已提交。
func (a *AggregateRoot) MarkCommitted() {
	a.uncommitted = nil
}

// HasUncommittedEvents 检查是否有未提交的事件。
func (a *AggregateRoot) HasUncommittedEvents() bool {
	return len(a.uncommitted) > 0
}

// EventApplier 事件应用器接口。
type EventApplier interface {
	// Apply 将事件应用到聚合根状态。
	Apply(event DomainEvent)
}

// Aggregate 完整聚合接口。
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

// LoadFromHistory 从事件历史恢复聚合根状态。
func LoadFromHistory(aggregate EventApplier, events []DomainEvent) {
	for _, event := range events {
		aggregate.Apply(event)
	}
}
