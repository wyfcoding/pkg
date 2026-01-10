package cache

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	redis_pkg "github.com/wyfcoding/pkg/redis"
	"github.com/wyfcoding/pkg/utils"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

var defaultCache *RedisCache

// Default 返回全局默认缓存实例
func Default() *RedisCache {
	return defaultCache
}

// SetDefault 设置全局默认缓存实例 (通常由构建框架调用)
func SetDefault(c *RedisCache) {
	defaultCache = c
}

// ErrCacheMiss 缓存未命中错误
var ErrCacheMiss = errors.New("cache: key not found")

// Cache 定义了工业级缓存的标准接口，支持基础操作、防击穿（GetOrSet）及优雅关闭。
type Cache interface {
	Get(ctx context.Context, key string, value any) error                                                        // 获取缓存，反序列化至 value
	Set(ctx context.Context, key string, value any, expiration time.Duration) error                              // 设置缓存，支持过期时间
	Delete(ctx context.Context, keys ...string) error                                                            // 删除一个或多个缓存键
	Exists(ctx context.Context, key string) (bool, error)                                                        // 判断键是否存在
	GetOrSet(ctx context.Context, key string, value any, expiration time.Duration, fn func() (any, error)) error // 解决缓存击穿的核心方法
	Close() error                                                                                                // 关闭缓存客户端资源
}

// RedisCache 是基于 Redis 实现的具体缓存结构
type RedisCache struct {
	client   redis.UniversalClient    // Redis 原生客户端 (通用接口)
	sfg      singleflight.Group       // 用于合并并发回源请求，防止击穿
	cleanup  func()                   // 资源清理回调函数
	cb       *breaker.Breaker         // 关联的熔断器，保护 Redis 不被过载
	logger   *logging.Logger          // 日志记录器
	hits     *prometheus.CounterVec   // 命中次数计数器
	misses   *prometheus.CounterVec   // 未命中次数计数器
	duration *prometheus.HistogramVec // 操作耗时分布
	prefix   string                   // 缓存 Key 的全局前缀
}

// NewRedisCache 初始化并返回一个具备熔断和监控能力的 Redis 缓存实例。
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
	hits := m.NewCounterVec(&prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "Total number of cache hits",
	}, []string{"service"})

	misses := m.NewCounterVec(&prometheus.CounterOpts{
		Name: "cache_misses_total",
		Help: "Total number of cache misses",
	}, []string{"service"})

	duration := m.NewHistogramVec(&prometheus.HistogramOpts{
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
		if errors.Is(err, redis.Nil) {
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
		var b [8]byte
		if _, err := rand.Read(b[:]); err == nil {
			mod := int64(expiration / 10)
			if mod > 0 {
				val := binary.LittleEndian.Uint64(b[:]) & 0x7FFFFFFFFFFFFFFF
				// 安全：val 和 mod 都是非负的，计算结果在 time.Duration 范围内。
				// G115 Fix: Ensure positive
				jitter := time.Duration(utils.Uint64ToInt64(val % uint64(mod)))
				expiration += jitter
			}
		}
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
			if err := c.Set(ctx, key, data, expiration); err != nil {
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

func (c *RedisCache) GetClient() redis.UniversalClient {
	return c.client
}
