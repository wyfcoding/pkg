// Package cache 提供了统一的缓存接口定义与多种后端实现。
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

// BigCache 是基于本地内存实现的高性能非阻塞缓存。
type BigCache struct { //nolint:govet
	cache  *bigcache.BigCache
	sfg    singleflight.Group
	logger *logging.Logger
}

// NewBigCache 初始化并返回一个新的本地内存缓存实例。
// 参数说明：
//   - ttl: 条目的默认过期时间。
//   - maxMB: 缓存占用的内存硬件软上限 (MB)。
func NewBigCache(ttl time.Duration, maxMB int, logger *logging.Logger) (*BigCache, error) {
	config := bigcache.Config{
		Shards:             1024,
		LifeWindow:         ttl,
		CleanWindow:        1 * time.Minute,
		MaxEntriesInWindow: 1000 * 10,
		MaxEntrySize:       500,
		Verbose:            false,
		HardMaxCacheSize:   maxMB,
		OnRemove:           nil,
		OnRemoveWithReason: nil,
	}

	bc, err := bigcache.New(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to init bigcache: %w", err)
	}

	return &BigCache{
		cache:  bc,
		logger: logger,
	}, nil
}

// Get 获取缓存，反序列化至传入的指针。
func (c *BigCache) Get(_ context.Context, key string, value any) error {
	data, err := c.cache.Get(key)
	if err != nil {
		if errors.Is(err, bigcache.ErrEntryNotFound) {
			return ErrCacheMiss
		}

		return err
	}

	return json.Unmarshal(data, value)
}

// Set 设置缓存，支持指定过期时间。
func (c *BigCache) Set(_ context.Context, key string, value any, _ time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return c.cache.Set(key, data)
}

// Delete 删除指定的缓存条目。
func (c *BigCache) Delete(_ context.Context, keys ...string) error {
	for _, key := range keys {
		if err := c.cache.Delete(key); err != nil && !errors.Is(err, bigcache.ErrEntryNotFound) {
			return err
		}
	}

	return nil
}

// Exists 判断指定键是否存在于缓存中。
func (c *BigCache) Exists(_ context.Context, key string) (bool, error) {
	_, err := c.cache.Get(key)
	if err == nil {
		return true, nil
	}

	if err == bigcache.ErrEntryNotFound {
		return false, nil
	}

	return false, err
}

// GetOrSet 实现防击穿逻辑：缓存失效时仅允许一个请求执行回源函数。
func (c *BigCache) GetOrSet(ctx context.Context, key string, value any, expiration time.Duration, fn func() (any, error)) error {
	if err := c.Get(ctx, key, value); err == nil {
		return nil
	}

	val, err, _ := c.sfg.Do(key, func() (any, error) {
		var inner any
		if getErr := c.Get(ctx, key, &inner); getErr == nil {
			return inner, nil
		}

		data, fnErr := fn()
		if fnErr != nil {
			return nil, fnErr
		}

		_ = c.Set(ctx, key, data, expiration)

		return data, nil
	})

	if err != nil {
		return err
	}

	raw, err := json.Marshal(val)
	if err != nil {
		return err
	}

	return json.Unmarshal(raw, value)
}

// Close 关闭缓存资源。
func (c *BigCache) Close() error {
	return c.cache.Close()
}
