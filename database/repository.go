// Package database 提供了增强的数据库访问能力.
package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/wyfcoding/pkg/xerrors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 定义了通用的仓储接口，支持基础 CRUD 操作.
type Repository[T any] interface {
	// Create 创建一个新实体.
	Create(ctx context.Context, entity *T) error
	// Update 更新实体信息.
	Update(ctx context.Context, entity *T) error
	// Upsert 创建或更新实体 (基于主键或唯一索引冲突).
	Upsert(ctx context.Context, entity *T) error
	// Delete 根据 ID 删除实体.
	Delete(ctx context.Context, id any) error
	// FindByID 根据 ID 查询单个实体.
	FindByID(ctx context.Context, id any) (*T, error)
	// FindAll 查询所有实体.
	FindAll(ctx context.Context) ([]*T, error)
	// Transaction 执行事务逻辑.
	Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error
}

// GormRepository 是基于 GORM 实现的通用仓储.
type GormRepository[T any] struct {
	db *gorm.DB
}

// NewGormRepository 创建一个新的 GORM 泛型仓储实例.
func NewGormRepository[T any](db *gorm.DB) *GormRepository[T] {
	return &GormRepository[T]{db: db}
}

// DB 返回底层的 GORM 实例.
func (r *GormRepository[T]) DB(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx)
}

// Create 实现 Repository.Create.
func (r *GormRepository[T]) Create(ctx context.Context, entity *T) error {
	if err := r.DB(ctx).Create(entity).Error; err != nil {
		return xerrors.WrapInternal(err, "failed to create entity")
	}
	return nil
}

// Update 实现 Repository.Update.
func (r *GormRepository[T]) Update(ctx context.Context, entity *T) error {
	if err := r.DB(ctx).Save(entity).Error; err != nil {
		return xerrors.WrapInternal(err, "failed to update entity")
	}
	return nil
}

// Upsert 实现 Repository.Upsert.
func (r *GormRepository[T]) Upsert(ctx context.Context, entity *T) error {
	if err := r.DB(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(entity).Error; err != nil {
		return xerrors.WrapInternal(err, "failed to upsert entity")
	}
	return nil
}

// Delete 实现 Repository.Delete.
func (r *GormRepository[T]) Delete(ctx context.Context, id any) error {
	var entity T
	if err := r.DB(ctx).Delete(&entity, id).Error; err != nil {
		return xerrors.WrapInternal(err, "failed to delete entity")
	}
	return nil
}

// FindByID 实现 Repository.FindByID.
func (r *GormRepository[T]) FindByID(ctx context.Context, id any) (*T, error) {
	var entity T
	if err := r.DB(ctx).First(&entity, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, xerrors.New(xerrors.ErrNotFound, 404, "entity not found", fmt.Sprintf("id: %v", id), err)
		}
		return nil, xerrors.WrapInternal(err, "failed to find entity by id")
	}
	return &entity, nil
}

// FindAll 实现 Repository.FindAll.
func (r *GormRepository[T]) FindAll(ctx context.Context) ([]*T, error) {
	var entities []*T
	if err := r.DB(ctx).Find(&entities).Error; err != nil {
		return nil, xerrors.WrapInternal(err, "failed to find all entities")
	}
	return entities, nil
}

// Transaction 实现 Repository.Transaction.
func (r *GormRepository[T]) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}
