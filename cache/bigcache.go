package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/wyfcoding/pkg/logging"
	"golang.org/x/sync/singleflight"
)

// BigCache 实现了 Cache 接口，利用底层分片内存技术提供高性能、零 GC 压力的本地缓存能力。
type BigCache struct {
	cache  *bigcache.BigCache // 底层 BigCache 实例
	sfg    singleflight.Group // 用于合并并发的回源请求，防止击穿
	logger *logging.Logger    // 日志记录器
}

// NewBigCache 初始化并返回一个新的本地高性能缓存实例。
// 参数说明：
//   - ttl: 条目的默认过期时间
//   - maxMB: 缓存占用的内存硬件软上限 (MB)
func NewBigCache(ttl time.Duration, maxMB int, logger *logging.Logger) (*BigCache, error) {
	config := bigcache.DefaultConfig(ttl)
	config.HardMaxCacheSize = maxMB
	config.CleanWindow = 5 * time.Minute

	cache, err := bigcache.New(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to init bigcache: %w", err)
	}

	logger.Info("local bigcache initialized successfully", "max_mb", maxMB, "default_ttl", ttl.String())
	return &BigCache{cache: cache, logger: logger}, nil
}

// Get 从本地内存中获取缓存数据并反序列化至 value 指针。
func (c *BigCache) Get(ctx context.Context, key string, value any) error {
	data, err := c.cache.Get(key)
	if err != nil {
		if errors.Is(err, bigcache.ErrEntryNotFound) {
			return ErrCacheMiss
		}
		return err
	}
	return json.Unmarshal(data, value)
}

func (c *BigCache) Set(ctx context.Context, key string, value any, _ time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.cache.Set(key, data)
}

// GetOrSet 实现防击穿逻辑
func (c *BigCache) GetOrSet(ctx context.Context, key string, value any, expiration time.Duration, fn func() (any, error)) error {
	err := c.Get(ctx, key, value)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrCacheMiss) {
		return err
	}

	v, err, _ := c.sfg.Do(key, func() (any, error) {
		var innerVal any
		if err := c.Get(ctx, key, &innerVal); err == nil {
			return innerVal, nil
		}

		data, err := fn()
		if err != nil {
			return nil, err
		}

		if err := c.Set(ctx, key, data, expiration); err != nil {
			c.logger.Error("failed to set cache in GetOrSet", "key", key, "error", err)
		}
		return data, nil
	})

	if err != nil {
		return err
	}

	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

func (c *BigCache) Delete(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		if err := c.cache.Delete(key); err != nil && !errors.Is(err, bigcache.ErrEntryNotFound) {
			c.logger.Error("failed to delete from bigcache", "key", key, "error", err)
		}
	}
	return nil
}

func (c *BigCache) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.cache.Get(key)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, bigcache.ErrEntryNotFound) {
		return false, nil
	}
	return false, err
}

func (c *BigCache) Close() error {
	return c.cache.Close()
}
