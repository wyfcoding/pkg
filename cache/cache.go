package cache

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"time"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	redis_pkg "github.com/wyfcoding/pkg/redis"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// ErrCacheMiss 缓存未命中错误
var ErrCacheMiss = errors.New("cache: key not found")

// Cache 工业级缓存接口
type Cache interface {
	Get(ctx context.Context, key string, value any) error
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)
	// GetOrSet 解决缓存击穿的核心方法
	GetOrSet(ctx context.Context, key string, value any, expiration time.Duration, fn func() (any, error)) error
	Close() error
}

type RedisCache struct {
	client  *redis.Client
	cleanup func()
	prefix  string
	cb      *breaker.Breaker
	sfg     singleflight.Group
	logger  *logging.Logger

	// 指标组件 (复用 pkg/metrics)
	hits     *prometheus.CounterVec
	misses   *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func NewRedisCache(cfg config.RedisConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) (*RedisCache, error) {
	client, cleanup, err := redis_pkg.NewClient(&cfg, logger)
	if err != nil {
		return nil, err
	}

	// 1. 复用 pkg/breaker
	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "redis-cache",
		Config: cbCfg,
	}, m)

	// 2. 复用 pkg/metrics 注册指标
	hits := m.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "Total number of cache hits",
	}, []string{"service"})

	misses := m.NewCounterVec(prometheus.CounterOpts{
		Name: "cache_misses_total",
		Help: "Total number of cache misses",
	}, []string{"service"})

	duration := m.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "cache_operation_duration_seconds",
		Help:    "Duration of cache operations",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "op"})

	return &RedisCache{
		client:   client,
		cleanup:  cleanup,
		cb:       cb,
		logger:   logger,
		hits:     hits,
		misses:   misses,
		duration: duration,
		prefix:   cfg.Addr, // 默认用地址做前缀标识
	}, nil
}

func (c *RedisCache) buildKey(key string) string {
	if c.prefix == "" {
		return key
	}
	return c.prefix + ":" + key
}

func (c *RedisCache) Get(ctx context.Context, key string, value any) error {
	start := time.Now()
	defer func() {
		c.duration.WithLabelValues(c.prefix, "get").Observe(time.Since(start).Seconds())
	}()

	// 这里的 Execute 提供了熔断保护，防止 Redis 故障拖垮服务
	_, err := c.cb.Execute(func() (any, error) {
		val, err := c.client.Get(ctx, c.buildKey(key)).Bytes()
		if err == redis.Nil {
			c.misses.WithLabelValues(c.prefix).Inc()
			return nil, ErrCacheMiss
		}
		if err != nil {
			return nil, err
		}
		c.hits.WithLabelValues(c.prefix).Inc()
		return val, json.Unmarshal(val, value)
	})

	return err
}

func (c *RedisCache) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	start := time.Now()
	defer func() {
		c.duration.WithLabelValues(c.prefix, "set").Observe(time.Since(start).Seconds())
	}()

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	// 注入随机扰动防止雪崩
	if expiration > 0 {
		jitter := time.Duration(rand.Int63n(int64(expiration / 10)))
		expiration = expiration + jitter
	}

	return c.client.Set(ctx, c.buildKey(key), data, expiration).Err()
}

func (c *RedisCache) GetOrSet(ctx context.Context, key string, value any, expiration time.Duration, fn func() (any, error)) error {
	// 1. 快速路径：缓存命中直接返回
	if err := c.Get(ctx, key, value); err == nil {
		return nil
	}

	// 2. 慢速路径：Singleflight 合并回源
	v, err, _ := c.sfg.Do(key, func() (any, error) {
		// 双重检查
		var innerVal any
		if err := c.Get(ctx, key, &innerVal); err == nil {
			return innerVal, nil
		}

		// 回源查询 (例如打库)
		data, err := fn()
		if err != nil {
			return nil, err
		}

		// 异步回写 (不阻塞 singleflight 释放)
		go func() {
			if err := c.Set(context.Background(), key, data, expiration); err != nil {
				c.logger.Error("cache backfill failed", "key", key, "error", err)
			}
		}()

		return data, nil
	})

	if err != nil {
		return err
	}

	// 将结果反序列化到传入的 value 指针
	// 注意：由于 singleflight 返回的是 any，我们需要处理类型转换
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	fullKeys := make([]string, len(keys))
	for i, k := range keys {
		fullKeys[i] = c.buildKey(k)
	}
	return c.client.Del(ctx, fullKeys...).Err()
}

func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, key).Result()
	return n > 0, err
}

func (c *RedisCache) Close() error {
	if c.cleanup != nil {
		c.cleanup()
	}
	return nil
}

func (c *RedisCache) GetClient() *redis.Client {
	return c.client
}
