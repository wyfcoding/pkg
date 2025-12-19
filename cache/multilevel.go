package cache

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// MultiLevelCache 实现多级缓存 (L1: 本地, L2: 分布式)。
type MultiLevelCache struct {
	l1     Cache // 一级缓存
	l2     Cache // 二级缓存
	tracer trace.Tracer
}

// NewMultiLevelCache 创建一个新的 MultiLevelCache 实例。
// l1 为一级缓存实例，l2 为二级缓存实例。
func NewMultiLevelCache(l1, l2 Cache) *MultiLevelCache {
	return &MultiLevelCache{
		l1:     l1,
		l2:     l2,
		tracer: otel.Tracer("github.com/wyfcoding/pkg/cache"),
	}
}

// Get 从多级缓存中获取一个键对应的值。
// 首先尝试从L1缓存获取，如果L1未命中，则尝试从L2获取，并将L2中的数据回填到L1。
func (c *MultiLevelCache) Get(ctx context.Context, key string, value interface{}) error {
	ctx, span := c.tracer.Start(ctx, "MultiLevelCache.Get", trace.WithAttributes(
		attribute.String("cache.key", key),
	))
	defer span.End()

	// 1. 尝试从一级缓存 (L1) 获取数据。
	if err := c.l1.Get(ctx, key, value); err == nil {
		span.SetAttributes(attribute.String("cache.hit", "L1"))
		return nil
	}

	// 2. 尝试从二级缓存 (L2) 获取数据。
	if err := c.l2.Get(ctx, key, value); err == nil {
		span.SetAttributes(attribute.String("cache.hit", "L2"))
		// L2命中后，将数据回填到L1缓存，以便后续请求可以直接从L1获取。
		_ = c.l1.Set(ctx, key, value, 0)
		return nil
	}

	// L1和L2都未命中，返回缓存未命中错误。
	span.SetAttributes(attribute.String("cache.hit", "miss"))
	return fmt.Errorf("缓存未命中: %s", key)
}

// Set 将一个键值对设置到多级缓存中。
// 数据会同时写入L1和L2缓存。
func (c *MultiLevelCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	ctx, span := c.tracer.Start(ctx, "MultiLevelCache.Set", trace.WithAttributes(
		attribute.String("cache.key", key),
	))
	defer span.End()

	// 1. 首先设置二级缓存 (L2)。
	if err := c.l2.Set(ctx, key, value, expiration); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to set L2")
		return fmt.Errorf("设置 L2 失败: %w", err)
	}

	// 2. 接着设置一级缓存 (L1)。即使L1设置失败，也不影响整个Set操作的成功。
	if err := c.l1.Set(ctx, key, value, expiration); err != nil {
		span.RecordError(err)
		// 如果 L1 失败，不要使操作失败，只需记录/追踪它。
		span.AddEvent("failed to set L1", trace.WithAttributes(attribute.String("error", err.Error())))
	}

	return nil
}

// Delete 从多级缓存中删除一个或多个键。
// 数据会从L1和L2缓存中同时删除，以确保数据一致性。
func (c *MultiLevelCache) Delete(ctx context.Context, keys ...string) error {
	ctx, span := c.tracer.Start(ctx, "MultiLevelCache.Delete", trace.WithAttributes(
		attribute.StringSlice("cache.keys", keys),
	))
	defer span.End()

	// 从L1和L2中删除键。
	err1 := c.l1.Delete(ctx, keys...)
	err2 := c.l2.Delete(ctx, keys...)

	if err1 != nil {
		span.RecordError(err1)
		return fmt.Errorf("删除 L1 失败: %w", err1)
	}
	if err2 != nil {
		span.RecordError(err2)
		return fmt.Errorf("删除 L2 失败: %w", err2)
	}
	return nil
}

// Exists 检查一个键是否存在于多级缓存中。
// 优先检查L1缓存，如果L1未命中则检查L2缓存。
func (c *MultiLevelCache) Exists(ctx context.Context, key string) (bool, error) {
	ctx, span := c.tracer.Start(ctx, "MultiLevelCache.Exists", trace.WithAttributes(
		attribute.String("cache.key", key),
	))
	defer span.End()

	// 首先检查 L1。
	exists, err := c.l1.Exists(ctx, key)
	if err != nil {
		return false, err
	}
	if exists {
		span.SetAttributes(attribute.Bool("cache.exists", true), attribute.String("cache.layer", "L1"))
		return true, nil
	}
	// 检查 L2。
	exists, err = c.l2.Exists(ctx, key)
	if err == nil {
		span.SetAttributes(attribute.Bool("cache.exists", exists), attribute.String("cache.layer", "L2"))
	}
	return exists, err
}

// Close 关闭多级缓存中的所有缓存层。
func (c *MultiLevelCache) Close() error {
	err1 := c.l1.Close()
	err2 := c.l2.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
