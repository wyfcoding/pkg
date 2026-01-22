// Package eventsourcing 提供事件溯源的核心抽象和基础设施。
package eventsourcing

import (
	"context"
	"errors"
	"fmt"
)

// ErrAggregateNotFound 聚合根未找到.
var ErrAggregateNotFound = errors.New("aggregate not found")

// AggregateRepository 定义了聚合根的泛型仓储接口。
type AggregateRepository[A Aggregate] interface {
	// Save 保存聚合根中的未提交事件。
	Save(ctx context.Context, aggregate A) error
	// Load 通过 ID 加载聚合根并恢复其状态。
	Load(ctx context.Context, id string) (A, error)
}

// EventSourcedRepository 是基于泛型的事件溯源仓储实现。
type EventSourcedRepository[A Aggregate] struct {
	store   EventStore
	factory func() A
}

// NewEventSourcedRepository 创建一个新的事件溯源仓储。
func NewEventSourcedRepository[A Aggregate](store EventStore, factory func() A) *EventSourcedRepository[A] {
	return &EventSourcedRepository[A]{
		store:   store,
		factory: factory,
	}
}

// Save 实现 AggregateRepository.Save。
func (r *EventSourcedRepository[A]) Save(ctx context.Context, aggregate A) error {
	events := aggregate.GetUncommittedEvents()
	if len(events) == 0 {
		return nil
	}

	// 期望版本为应用新事件之前的版本
	expectedVersion := aggregate.Version() - int64(len(events))
	if err := r.store.Save(ctx, aggregate.ID(), events, expectedVersion); err != nil {
		return fmt.Errorf("failed to save events to store: %w", err)
	}

	aggregate.MarkCommitted()

	return nil
}

// Load 实现 AggregateRepository.Load。
func (r *EventSourcedRepository[A]) Load(ctx context.Context, id string) (A, error) {
	aggregate := r.factory()
	aggregate.SetID(id)

	// 1. 尝试加载快照
	snapshot, version, err := r.store.GetSnapshot(ctx, id)
	if err == nil && snapshot != nil {
		if applier, ok := any(aggregate).(interface{ ApplySnapshot(snapshot any) }); ok {
			applier.ApplySnapshot(snapshot)
			aggregate.SetVersion(version)
		}
	}

	// 2. 加载快照之后的事件（如果没有快照则加载所有事件）
	events, err := r.store.LoadFromVersion(ctx, id, aggregate.Version()+1)
	if err != nil {
		var zero A
		return zero, fmt.Errorf("failed to load events from store: %w", err)
	}

	if len(events) == 0 && aggregate.Version() == 0 {
		var zero A
		return zero, fmt.Errorf("%w: %s", ErrAggregateNotFound, id)
	}

	// 3. 回放事件恢复状态
	LoadFromHistory(aggregate, events)

	return aggregate, nil
}
