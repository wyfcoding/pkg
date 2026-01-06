package gormstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wyfcoding/pkg/eventsourcing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EventModel 数据库持久化模型，记录每一条领域事件。
type EventModel struct {
	gorm.Model
	AggregateID string    `gorm:"type:varchar(64);index:idx_agg_ver,unique;not null;comment:聚合根唯一标识"`
	Type        string    `gorm:"type:varchar(128);not null;comment:事件类型名称"`
	Version     int64     `gorm:"index:idx_agg_ver,unique;not null;comment:事件在流中的版本号"`
	Data        string    `gorm:"type:json;not null;comment:事件载荷数据 (JSON 格式)"`
	Metadata    string    `gorm:"type:json;comment:事件上下文元数据"`
	OccurredAt  time.Time `gorm:"index;not null;comment:业务事件发生时间"`
}

// SnapshotModel 快照持久化模型，存储聚合根的最新状态副本。
type SnapshotModel struct {
	gorm.Model
	AggregateID string `gorm:"type:varchar(64);uniqueIndex;not null;comment:聚合根唯一标识"`
	Version     int64  `gorm:"not null;comment:快照对应的聚合版本"`
	State       string `gorm:"type:json;not null;comment:聚合根状态数据 (JSON 格式)"`
}

// GormEventStore 基于 GORM 的 EventStore 实现
type GormEventStore struct {
	db        *gorm.DB
	tableName string
}

// NewGormEventStore 创建新的 GORM EventStore
// db: GORM 数据库连接
// tableName: 事件表名（默认为 "events"）
func NewGormEventStore(db *gorm.DB, tableName string) (*GormEventStore, error) {
	if tableName == "" {
		tableName = "events"
	}
	store := &GormEventStore{
		db:        db,
		tableName: tableName,
	}

	// 自动迁移
	// 注意：在生产环境建议通过专门的 migration 工具管理
	if err := db.Table(tableName).AutoMigrate(&EventModel{}); err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&SnapshotModel{}); err != nil {
		return nil, err
	}

	return store, nil
}

// Save 实现 EventStore 接口，在事务中保存事件流。
func (s *GormEventStore) Save(ctx context.Context, aggregateID string, events []eventsourcing.DomainEvent, expectedVersion int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 1. 乐观锁检查
		var currentVersion int64
		err := tx.Table(s.tableName).
			Where("aggregate_id = ? AND deleted_at IS NULL", aggregateID).
			Select("COALESCE(MAX(version), 0)").
			Scan(&currentVersion).Error
		if err != nil {
			return err
		}

		if currentVersion != expectedVersion {
			return fmt.Errorf("concurrency error: aggregate %s version mismatch, expected %d but got %d", aggregateID, expectedVersion, currentVersion)
		}

		// 2. 批量插入
		eventModels := make([]*EventModel, 0, len(events))
		for i, event := range events {
			// 简单序列化 Data
			// 注意：这里假设 event 本身（或其 Data 字段）可以被 JSON 序列化
			// 在 BaseEvent 中，Data 是 any 类型

			// 我们需要从 event 中提取实际的数据载荷
			var dataAny any
			var metaAny any

			if baseEventPtr, ok := event.(*eventsourcing.BaseEvent); ok {
				dataAny = baseEventPtr.Data
				metaAny = baseEventPtr.Metadata
			} else {
				// 如果是其他自定义实现了 DomainEvent 接口的结构体，尝试将整个事件作为数据
				// 或者需要反射/接口断言来获取数据部分。
				// 为简化，这里直接序列化整个事件对象，但这通常不是最佳实践（包含冗余字段）
				// 更好的做法是定义 Payload() 方法在 DomainEvent 接口中
				dataAny = event
			}

			dataBytes, err := json.Marshal(dataAny)
			if err != nil {
				return fmt.Errorf("failed to marshal event data: %w", err)
			}

			metaBytes, err := json.Marshal(metaAny)
			if err != nil {
				return fmt.Errorf("failed to marshal event metadata: %w", err)
			}

			eventModels = append(eventModels, &EventModel{
				AggregateID: aggregateID,
				Type:        event.EventType(),
				Version:     expectedVersion + int64(i) + 1,
				Data:        string(dataBytes),
				Metadata:    string(metaBytes),
				OccurredAt:  event.OccurredAt(),
			})
		}

		if len(eventModels) == 0 {
			return nil
		}

		return tx.Table(s.tableName).Create(&eventModels).Error
	})
}

// Load 实现 EventStore 接口
func (s *GormEventStore) Load(ctx context.Context, aggregateID string) ([]eventsourcing.DomainEvent, error) {
	return s.LoadFromVersion(ctx, aggregateID, 0)
}

// LoadFromVersion 实现 EventStore 接口
func (s *GormEventStore) LoadFromVersion(ctx context.Context, aggregateID string, fromVersion int64) ([]eventsourcing.DomainEvent, error) {
	var models []EventModel
	err := s.db.Table(s.tableName).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Order("version ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}

	events := make([]eventsourcing.DomainEvent, 0, len(models))
	for _, model := range models {
		// 反序列化
		// 这里的难点是反序列化回具体的事件类型（如 OrderCreatedEvent）
		// 通用存储层无法知道具体类型。
		// 通常需要一个 EventRegistry 或 EventFactory。
		// 在这个简单实现中，我们返回一个通用的 BaseEvent，
		// 其中的 Data 字段保持为 map[string]any 或者 json.RawMessage，
		// 由 Application 层负责进一步解析。

		var dataMap map[string]any
		if err := json.Unmarshal([]byte(model.Data), &dataMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event data: %w", err)
		}

		var metaMap eventsourcing.Metadata
		if len(model.Metadata) > 0 {
			if err := json.Unmarshal([]byte(model.Metadata), &metaMap); err != nil {
				// 元数据解析失败仅记录日志，不中断主流程
				fmt.Printf("warning: failed to unmarshal metadata for event %d: %v\n", model.ID, err)
			}
		}

		event := eventsourcing.BaseEvent{
			ID:          fmt.Sprintf("%d", model.ID),
			Type:        model.Type,
			AggregateId: model.AggregateID,
			Ver:         model.Version,
			Timestamp:   model.OccurredAt,
			Data:        dataMap,
			Metadata:    metaMap,
		}
		events = append(events, &event)
	}

	return events, nil
}

// SaveSnapshot 实现 EventStore 接口
func (s *GormEventStore) SaveSnapshot(ctx context.Context, aggregateID string, state any, version int64) error {
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return err
	}

	snapshot := SnapshotModel{
		AggregateID: aggregateID,
		Version:     version,
		State:       string(stateBytes),
	}

	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "aggregate_id"}}, // 假设每个聚合只有一个最新快照
		DoUpdates: clause.AssignmentColumns([]string{"version", "state", "updated_at"}),
	}).Create(&snapshot).Error
}

// GetSnapshot 实现 EventStore 接口
func (s *GormEventStore) GetSnapshot(ctx context.Context, aggregateID string) (any, int64, error) {
	var snapshot SnapshotModel
	err := s.db.Where("aggregate_id = ?", aggregateID).First(&snapshot).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, 0, nil
		}
		return nil, 0, err
	}

	// 同样，只能返回 map[string]any 或 raw json，由业务层转换
	var stateMap map[string]any
	if err := json.Unmarshal([]byte(snapshot.State), &stateMap); err != nil {
		return nil, 0, err
	}

	return stateMap, snapshot.Version, nil
}
