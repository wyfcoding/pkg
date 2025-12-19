package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	redis_pkg "github.com/wyfcoding/pkg/redis"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
)

var (
	// cacheHits is a Prometheus counter for cache hits
	cacheHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "The total number of cache hits",
		},
		[]string{"prefix"},
	)
	// cacheMisses is a Prometheus counter for cache misses
	cacheMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "The total number of cache misses",
		},
		[]string{"prefix"},
	)
	// cacheDuration is a Prometheus histogram for cache operation duration
	cacheDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cache_operation_duration_seconds",
			Help:    "The duration of cache operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"prefix", "operation"},
	)
)

// init registers Prometheus metrics
func init() {
	prometheus.MustRegister(cacheHits, cacheMisses, cacheDuration)
}

// Cache defines the cache interface
type Cache interface {
	Get(ctx context.Context, key string, value interface{}) error
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	Close() error
}

// RedisCache implements Cache using Redis
type RedisCache struct {
	client  *redis.Client             // Redis client instance
	cleanup func()                    // Cleanup function to close the client
	prefix  string                    // Cache key prefix
	cb      *gobreaker.CircuitBreaker // Circuit breaker instance
}

// NewRedisCache creates a new RedisCache instance
func NewRedisCache(cfg config.RedisConfig) (*RedisCache, error) {
	// Use shared Redis client factory
	client, cleanup, err := redis_pkg.NewClient(&cfg, logging.Default())
	if err != nil {
		return nil, err
	}

	// Initialize circuit breaker
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "redis-cache",
		MaxRequests: 0,
		Interval:    0,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 10 && failureRatio >= 0.6
		},
	})

	return &RedisCache{
		client:  client,
		cleanup: cleanup,
		prefix:  "", // Default no prefix
		cb:      cb,
	}, nil
}

// WithPrefix returns a new RedisCache with a key prefix
// The underlying client is shared
func (c *RedisCache) WithPrefix(prefix string) *RedisCache {
	return &RedisCache{
		client:  c.client,
		cleanup: c.cleanup, // Share cleanup? Careful here. Usually cleanup belongs to the owner.
		prefix:  prefix,
		cb:      c.cb,
	}
}

// buildKey 构建带有前缀的 key。
func (c *RedisCache) buildKey(key string) string {
	if c.prefix == "" {
		return key
	}
	return c.prefix + ":" + key
}

// Get 从缓存中获取值。
// value 参数必须是一个指针，以便能将缓存的数据反序列化到其中。
func (c *RedisCache) Get(ctx context.Context, key string, value interface{}) error {
	start := time.Now()
	defer func() {
		cacheDuration.WithLabelValues(c.prefix, "get").Observe(time.Since(start).Seconds())
	}()

	fullKey := c.buildKey(key)

	// 使用熔断器包装Redis的Get操作。
	_, err := c.cb.Execute(func() (interface{}, error) {
		data, err := c.client.Get(ctx, fullKey).Bytes()
		if err != nil {
			if err == redis.Nil {
				cacheMisses.WithLabelValues(c.prefix).Inc()
				return nil, fmt.Errorf("缓存未命中: %s", key)
			}
			return nil, err
		}
		cacheHits.WithLabelValues(c.prefix).Inc()
		return data, json.Unmarshal(data, value)
	})

	return err
}

// Set 设置缓存值。
// value 会被JSON序列化后存储。
// expiration 参数指定了键的过期时间。
func (c *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	start := time.Now()
	defer func() {
		cacheDuration.WithLabelValues(c.prefix, "set").Observe(time.Since(start).Seconds())
	}()

	fullKey := c.buildKey(key)
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	// 使用熔断器包装Redis的Set操作。
	_, err = c.cb.Execute(func() (interface{}, error) {
		return nil, c.client.Set(ctx, fullKey, data, expiration).Err()
	})

	return err
}

// Delete 从缓存中删除值。
func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	start := time.Now()
	defer func() {
		cacheDuration.WithLabelValues(c.prefix, "delete").Observe(time.Since(start).Seconds())
	}()

	if len(keys) == 0 {
		return nil
	}
	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = c.buildKey(key)
	}

	// 使用熔断器包装Redis的Del操作。
	_, err := c.cb.Execute(func() (interface{}, error) {
		return nil, c.client.Del(ctx, fullKeys...).Err()
	})

	return err
}

// Exists 检查 key 是否存在。
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	start := time.Now()
	defer func() {
		cacheDuration.WithLabelValues(c.prefix, "exists").Observe(time.Since(start).Seconds())
	}()

	fullKey := c.buildKey(key)

	// 使用熔断器包装Redis的Exists操作。
	result, err := c.cb.Execute(func() (interface{}, error) {
		n, err := c.client.Exists(ctx, fullKey).Result()
		if err != nil {
			return false, err
		}
		return n > 0, nil
	})

	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

// Close 关闭 Redis 客户端。
func (c *RedisCache) Close() error {
	slog.Info("正在关闭 Redis 缓存连接...")
	if c.cleanup != nil {
		c.cleanup()
	}
	return nil
}

// GetClient 返回底层的 Redis 客户端。
// 允许直接访问Redis客户端以执行Cache接口未封装的更高级操作。
func (c *RedisCache) GetClient() *redis.Client {
	return c.client
}
