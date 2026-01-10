// Package gormstore 提供了基于 GORM 的事件存储实现.
package gormstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/wyfcoding/pkg/eventsourcing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrConcurrency 乐观锁并发冲突.
	ErrConcurrency = errors.New("concurrency error: version mismatch")
	// ErrLoadEvents 加载事件失败.
	ErrLoadEvents = errors.New("failed to load events")
	// ErrSnapshot 快照操作失败.
	ErrSnapshot = errors.New("snapshot operation failed")
)

// EventModel 数据库持久化模型.
type EventModel struct {
	OccurredAt  time.Time `gorm:"index;not null;comment:业务事件发生时间"`
	AggregateID string    `gorm:"type:varchar(64);index:idx_agg_ver,unique;not null;comment:聚合根唯一标识"`
	Type        string    `gorm:"type:varchar(128);not null;comment:事件类型名称"`
	Data        string    `gorm:"type:json;not null;comment:事件载荷数据 (JSON 格式)"`
	Metadata    string    `gorm:"type:json;comment:事件上下文元数据"`
	gorm.Model
	Version int64 `gorm:"index:idx_agg_ver,unique;not null;comment:事件在流中的版本号"`
}

// SnapshotModel 快照持久化模型.
type SnapshotModel struct {
	AggregateID string `gorm:"type:varchar(64);uniqueIndex;not null;comment:聚合根唯一标识"`
	State       string `gorm:"type:json;not null;comment:聚合根状态数据 (JSON 格式)"`
	gorm.Model
	Version int64 `gorm:"not null;comment:快照对应的聚合版本"`
}

// GormEventStore 基于 GORM 的 EventStore 实现.
type GormEventStore struct {
	db        *gorm.DB
	tableName string
}

// NewGormEventStore 创建新的 GORM EventStore.
func NewGormEventStore(db *gorm.DB, tableName string) (*GormEventStore, error) {
	name := tableName
	if name == "" {
		name = "events"
	}

	store := &GormEventStore{
		db:        db,
		tableName: name,
	}

	if err := db.Table(name).AutoMigrate(&EventModel{}); err != nil {
		return nil, fmt.Errorf("failed to migrate events table: %w", err)
	}

	if err := db.AutoMigrate(&SnapshotModel{}); err != nil {
		return nil, fmt.Errorf("failed to migrate snapshots table: %w", err)
	}

	return store, nil
}

// Save 在事务中保存事件流.
func (s *GormEventStore) Save(ctx context.Context, aggregateID string, events []eventsourcing.DomainEvent, expectedVersion int64) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var currentVersion int64
		err := tx.Table(s.tableName).
			Where("aggregate_id = ? AND deleted_at IS NULL", aggregateID).
			Select("COALESCE(MAX(version), 0)").
			Scan(&currentVersion).Error
		if err != nil {
			return fmt.Errorf("failed to query max version: %w", err)
		}

		if currentVersion != expectedVersion {
			return fmt.Errorf("%w: aggregate %s, expected %d but got %d",
				ErrConcurrency, aggregateID, expectedVersion, currentVersion)
		}

		eventModels := make([]*EventModel, 0, len(events))
		for idx, event := range events {
			var dataAny any
			var metaAny any

			if baseEventPtr, ok := event.(*eventsourcing.BaseEvent); ok {
				dataAny = baseEventPtr.Data
				metaAny = baseEventPtr.Metadata
			} else {
				dataAny = event
			}

			dataBytes, errMarshal := json.Marshal(dataAny)
			if errMarshal != nil {
				return fmt.Errorf("failed to marshal event data: %w", errMarshal)
			}

			metaBytes, errMeta := json.Marshal(metaAny)
			if errMeta != nil {
				return fmt.Errorf("failed to marshal event metadata: %w", errMeta)
			}

			eventModels = append(eventModels, &EventModel{
				AggregateID: aggregateID,
				Type:        event.EventType(),
				Version:     expectedVersion + int64(idx) + 1,
				Data:        string(dataBytes),
				Metadata:    string(metaBytes),
				OccurredAt:  event.OccurredAt(),
				Model:       gorm.Model{}, //nolint:exhaustruct // 经过审计，此处忽略是安全的。
			})
		}

		if len(eventModels) == 0 {
			return nil
		}

		return tx.WithContext(ctx).Table(s.tableName).Create(&eventModels).Error
	})
}

// Load 加载所有事件.
func (s *GormEventStore) Load(ctx context.Context, aggregateID string) ([]eventsourcing.DomainEvent, error) {
	return s.LoadFromVersion(ctx, aggregateID, 0)
}

// LoadFromVersion 从指定版本加载事件.
func (s *GormEventStore) LoadFromVersion(ctx context.Context, aggregateID string, fromVersion int64) ([]eventsourcing.DomainEvent, error) {
	var models []EventModel
	err := s.db.WithContext(ctx).Table(s.tableName).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Order("version ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoadEvents, err)
	}

	events := make([]eventsourcing.DomainEvent, 0, len(models))
	for i := range models {
		model := &models[i]
		var dataMap map[string]any
		if errData := json.Unmarshal([]byte(model.Data), &dataMap); errData != nil {
			return nil, fmt.Errorf("failed to unmarshal event data: %w", errData)
		}

		var metaMap eventsourcing.Metadata
		if model.Metadata != "" {
			if errMeta := json.Unmarshal([]byte(model.Metadata), &metaMap); errMeta != nil {
				slog.WarnContext(ctx, "failed to unmarshal metadata", "id", model.ID, "error", errMeta)
			}
		}

		event := eventsourcing.BaseEvent{
			ID:        strconv.FormatUint(uint64(model.ID), 10),
			Type:      model.Type,
			AggID:     model.AggregateID,
			Ver:       model.Version,
			Timestamp: model.OccurredAt,
			Data:      dataMap,
			Metadata:  metaMap,
		}
		events = append(events, &event)
	}

	return events, nil
}

// SaveSnapshot 保存聚合快照.
func (s *GormEventStore) SaveSnapshot(ctx context.Context, aggregateID string, state any, version int64) error {
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot state: %w", err)
	}

	snapshot := SnapshotModel{
		AggregateID: aggregateID,
		Version:     version,
		State:       string(stateBytes),
		Model:       gorm.Model{}, //nolint:exhaustruct // 经过审计，此处忽略是安全的。
	}

	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:      []clause.Column{{Name: "aggregate_id", Table: "", Alias: "", Raw: false}},
		DoUpdates:    clause.AssignmentColumns([]string{"version", "state", "updated_at"}),
		Where:        clause.Where{},
		OnConstraint: "",
		DoNothing:    false,
		UpdateAll:    false,
	}).Create(&snapshot).Error
}

// GetSnapshot 获取最新快照.
func (s *GormEventStore) GetSnapshot(ctx context.Context, aggregateID string) (state any, version int64, err error) {
	var snapshot SnapshotModel
	errDB := s.db.WithContext(ctx).Where("aggregate_id = ?", aggregateID).First(&snapshot).Error
	if errDB != nil {
		if errors.Is(errDB, gorm.ErrRecordNotFound) {
			return nil, 0, nil
		}

		return nil, 0, fmt.Errorf("%w: %w", ErrSnapshot, errDB)
	}

	var stateMap map[string]any
	if errUnmarshal := json.Unmarshal([]byte(snapshot.State), &stateMap); errUnmarshal != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal snapshot state: %w", errUnmarshal)
	}

	return stateMap, snapshot.Version, nil
}
