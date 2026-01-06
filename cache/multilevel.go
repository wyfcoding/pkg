package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/wyfcoding/pkg/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// MultiLevelCache 实现了多级缓存策略，结合了 L1（低延迟、零网络开销的本地内存）与 L2（高可用、数据共享的分布式 Redis）。
// 关键特性：具备自动回填（Backfill）能力及全链路分布式追踪集成。
type MultiLevelCache struct {
	l1     Cache         // 本地一级缓存 (如 BigCache)
	l2     Cache         // 分布式二级缓存 (如 RedisCache)
	tracer trace.Tracer  // 分布式追踪器
	logger *logging.Logger // 结构化日志记录器
}

// NewMultiLevelCache 初始化并返回一个新的多级缓存管理器。
func NewMultiLevelCache(l1, l2 Cache, logger *logging.Logger) *MultiLevelCache {
	logger.Info("multi-level cache initialized", "strategy", "L1_L2_Coordination")
	return &MultiLevelCache{
		l1:     l1,
		l2:     l2,
		tracer: otel.Tracer("github.com/wyfcoding/pkg/cache"),
		logger: logger,
	}
}

// Get 依次尝试从各级缓存中检索数据。
// 流程：L1 命中直接返回 -> L2 命中则回填 L1 并返回 -> 全不中返回 ErrCacheMiss。
func (c *MultiLevelCache) Get(ctx context.Context, key string, value any) error {
	ctx, span := c.tracer.Start(ctx, "multi_level_cache.get", trace.WithAttributes(
		attribute.String("cache.key", key),
	))
	defer span.End()

	// 1. 尝试 L1 (Local)
	if err := c.l1.Get(ctx, key, value); err == nil {
		span.SetAttributes(attribute.String("cache.hit", "L1"))
		return nil
	}

	// 2. 尝试 L2 (Distributed)
	if err := c.l2.Get(ctx, key, value); err == nil {
		span.SetAttributes(attribute.String("cache.hit", "L2"))
		// 将结果自动同步回 L1，加速下一次访问
		if err := c.l1.Set(ctx, key, value, 0); err != nil {
			c.logger.ErrorContext(ctx, "failed to backfill L1 cache", "key", key, "error", err)
		}
		return nil
	}

	span.SetAttributes(attribute.String("cache.hit", "miss"))
	return ErrCacheMiss
}

// Set 同时更新各级缓存，确保数据一致性。
// 策略：先写 L2（确保全局可见），再写 L1（单机可见）。
func (c *MultiLevelCache) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	ctx, span := c.tracer.Start(ctx, "multi_level_cache.set", trace.WithAttributes(
		attribute.String("cache.key", key),
	))
	defer span.End()

	if err := c.l2.Set(ctx, key, value, expiration); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to set L2 cache")
		return fmt.Errorf("failed to set L2: %w", err)
	}

	if err := c.l1.Set(ctx, key, value, expiration); err != nil {
		c.logger.ErrorContext(ctx, "failed to set L1 cache", "key", key, "error", err)
	}
	return nil
}

// GetOrSet 多级缓存下的防击穿逻辑
func (c *MultiLevelCache) GetOrSet(ctx context.Context, key string, value any, expiration time.Duration, fn func() (any, error)) error {
	// 1. 先尝试获取
	err := c.Get(ctx, key, value)
	if err == nil {
		return nil
	}

	// 2. 只有在 L1 和 L2 都没中时才去回源
	// 注意：为了防击穿，我们直接调用 L2 的 GetOrSet，因为 L2 (Redis) 通常已经具备分布式防击穿能力
	// 或者是通过 Singleflight 保护 L2 后的回源操作
	return c.l2.GetOrSet(ctx, key, value, expiration, func() (any, error) {
		res, err := fn()
		if err != nil {
			return nil, err
		}
		// 回源成功后，Set 会自动写 L2，我们这里只需确保 fn 执行成功
		return res, nil
	})
}

func (c *MultiLevelCache) Delete(ctx context.Context, keys ...string) error {
	if err := c.l1.Delete(ctx, keys...); err != nil {
		c.logger.ErrorContext(ctx, "failed to delete from L1 cache", "keys", keys, "error", err)
	}
	return c.l2.Delete(ctx, keys...)
}

func (c *MultiLevelCache) Exists(ctx context.Context, key string) (bool, error) {
	exists, err := c.l1.Exists(ctx, key)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to check L1 cache existence", "key", key, "error", err)
	}
	if exists {
		return true, nil
	}
	return c.l2.Exists(ctx, key)
}

func (c *MultiLevelCache) Close() error {
	var err error
	if l1Err := c.l1.Close(); l1Err != nil {
		c.logger.Error("failed to close L1 cache", "error", l1Err)
		err = l1Err
	}
	if l2Err := c.l2.Close(); l2Err != nil {
		c.logger.Error("failed to close L2 cache", "error", l2Err)
		err = l2Err
	}
	return err
}
