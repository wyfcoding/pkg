// Package cache 提供了缓存抽象和多种缓存实现，包括多级缓存、本地缓存和分布式缓存。
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/allegro/bigcache/v3" // 导入高性能本地缓存库
)

// BigCache 实现了 `Cache` 接口，使用 `allegro/bigcache` 作为底层存储。
// BigCache是一个高性能、支持并发、基于内存的本地缓存，适用于存储大量数据。
type BigCache struct {
	cache *bigcache.BigCache // 底层的BigCache实例
}

// NewBigCache 创建并返回一个新的 BigCache 实例。
// ttl: 缓存项的全局过期时间（Time To Live）。BigCache对所有项统一设置过期时间。
// maxMB: 缓存的最大容量（单位MB），用于控制内存使用。
func NewBigCache(ttl time.Duration, maxMB int) (*BigCache, error) {
	// 配置BigCache，设置全局TTL和最大内存使用。
	config := bigcache.DefaultConfig(ttl)
	config.HardMaxCacheSize = maxMB      // 设置硬性最大缓存大小（MB）
	config.CleanWindow = 5 * time.Minute // 垃圾回收周期，BigCache会在此周期内清理过期项。

	cache, err := bigcache.New(context.Background(), config) // 初始化BigCache
	if err != nil {
		return nil, fmt.Errorf("初始化 bigcache 失败: %w", err)
	}

	return &BigCache{cache: cache}, nil
}

// Get 从BigCache中获取指定键的值。
// value 参数必须是一个指针，缓存的数据会反序列化到其中。
func (c *BigCache) Get(ctx context.Context, key string, value interface{}) error {
	data, err := c.cache.Get(key) // 从底层BigCache获取原始字节数据
	if err != nil {
		if err == bigcache.ErrEntryNotFound {
			return fmt.Errorf("缓存未命中: %s", key) // 如果未找到，返回缓存未命中错误
		}
		return err // 其他错误直接返回
	}
	return json.Unmarshal(data, value) // 将获取到的字节数据反序列化为指定类型
}

// Set 将一个键值对设置到BigCache中。
// value 会被JSON序列化后存储。
// 注意：BigCache不支持为每个单独的键设置过期时间，它使用 `NewBigCache` 中定义的全局TTL。
// 因此，这里的 `expiration` 参数将被忽略。
func (c *BigCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	// 将值序列化为JSON字节数据
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.cache.Set(key, data) // 存储键值对
}

// Delete 从BigCache中删除一个或多个键。
// 如果键不存在，不会返回错误。
func (c *BigCache) Delete(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		if err := c.cache.Delete(key); err != nil {
			// 忽略“未找到”错误，只处理其他真正的删除错误
			if err != bigcache.ErrEntryNotFound {
				return err
			}
		}
	}
	return nil
}

// Exists 检查BigCache中是否存在指定的键。
func (c *BigCache) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.cache.Get(key) // 尝试获取键
	if err == nil {
		return true, nil // 如果fetched successfully，表示存在
	}
	if err == bigcache.ErrEntryNotFound {
		return false, nil // 如果返回未找到错误，表示不存在
	}
	return false, err // 其他错误直接返回
}

// Close 关闭BigCache实例，释放其占用的资源。
func (c *BigCache) Close() error {
	return c.cache.Close()
}
