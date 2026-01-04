package eventsourcing

import "context"

// EventStore 事件存储接口
// 定义了事件的持久化和检索操作
type EventStore interface {
	// Save 保存事件到指定聚合的事件流
	// ctx: 上下文
	// aggregateID: 聚合根ID
	// events: 要保存的领域事件列表
	// expectedVersion: 期望的当前版本号（用于乐观并发控制）
	// 返回 error 如果版本冲突或存储失败
	Save(ctx context.Context, aggregateID string, events []DomainEvent, expectedVersion int64) error

	// Load 加载指定聚合的所有事件
	// ctx: 上下文
	// aggregateID: 聚合根ID
	// 返回 历史事件列表 和 error
	Load(ctx context.Context, aggregateID string) ([]DomainEvent, error)

	// LoadFromVersion 从指定版本开始加载事件
	// ctx: 上下文
	// aggregateID: 聚合根ID
	// fromVersion: 起始版本号（包含）
	// 返回 事件列表 和 error
	LoadFromVersion(ctx context.Context, aggregateID string, fromVersion int64) ([]DomainEvent, error)

	// GetSnapshot 获取聚合快照
	// ctx: 上下文
	// aggregateID: 聚合根ID
	// 返回 快照状态对象, 快照版本号, error
	GetSnapshot(ctx context.Context, aggregateID string) (any, int64, error)

	// SaveSnapshot 保存聚合快照
	// ctx: 上下文
	// aggregateID: 聚合根ID
	// state: 快照状态对象
	// version: 快照对应的版本号
	// 返回 error
	SaveSnapshot(ctx context.Context, aggregateID string, state any, version int64) error
}

// SnapshotStrategy 快照策略接口
type SnapshotStrategy interface {
	// ShouldSnapshot 判断是否应该创建快照
	// aggregate: 聚合根
	// eventsLen: 本次新增事件数量
	ShouldSnapshot(aggregate Aggregate, eventsLen int) bool
}

// DefaultSnapshotStrategy 默认快照策略（基于版本间隔）
type DefaultSnapshotStrategy struct {
	Interval int64
}

// NewDefaultSnapshotStrategy 创建基于间隔的快照策略
func NewDefaultSnapshotStrategy(interval int64) *DefaultSnapshotStrategy {
	return &DefaultSnapshotStrategy{Interval: interval}
}

func (s *DefaultSnapshotStrategy) ShouldSnapshot(aggregate Aggregate, eventsLen int) bool {
	// 简单的策略：如果当前版本是间隔的倍数，则快照
	// 注意：这只是一个近似策略，实际可能需要更复杂的判断逻辑
	return aggregate.Version()%s.Interval == 0
}
