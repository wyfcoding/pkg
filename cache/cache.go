package cache

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"time"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	redis_pkg "github.com/wyfcoding/pkg/redis"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
	"golang.org/x/sync/singleflight"
)

var (
	cacheHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "cache_hits_total", Help: "缓存命中总数"},
		[]string{"prefix"},
	)
	cacheMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "cache_misses_total", Help: "缓存未命中总数"},
		[]string{"prefix"},
	)
	cacheDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cache_operation_duration_seconds",
			Help:    "缓存操作耗时",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"prefix", "operation"},
	)
)

func init() {
	prometheus.MustRegister(cacheHits, cacheMisses, cacheDuration)
}

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
	cb      *gobreaker.CircuitBreaker
	sfg     singleflight.Group
}

func NewRedisCache(cfg config.RedisConfig) (*RedisCache, error) {
	client, cleanup, err := redis_pkg.NewClient(&cfg, logging.Default())
	if err != nil {
		return nil, err
	}

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:    "redis-cache",
		Timeout: 10 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.Requests >= 20 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.5
		},
	})

	return &RedisCache{
		client:  client,
		cleanup: cleanup,
		cb:      cb,
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
		cacheDuration.WithLabelValues(c.prefix, "get").Observe(time.Since(start).Seconds())
	}()

	fullKey := c.buildKey(key)

	val, err := c.client.Get(ctx, fullKey).Bytes()
	if err == redis.Nil {
		cacheMisses.WithLabelValues(c.prefix).Inc()
		return ErrCacheMiss
	} else if err != nil {
		return err
	}

	cacheHits.WithLabelValues(c.prefix).Inc()
	return json.Unmarshal(val, value)
}

// Set 存入缓存，并增加随机 TTL 扰动防止雪崩
func (c *RedisCache) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	start := time.Now()
	defer func() {
		cacheDuration.WithLabelValues(c.prefix, "set").Observe(time.Since(start).Seconds())
	}()

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	// 注入 10% 以内的随机扰动
	if expiration > 0 {
		jitter := time.Duration(rand.Int63n(int64(expiration / 10)))
		expiration = expiration + jitter
	}

	return c.client.Set(ctx, c.buildKey(key), data, expiration).Err()
}

// GetOrSet 实现“防击穿”逻辑
func (c *RedisCache) GetOrSet(ctx context.Context, key string, value any, expiration time.Duration, fn func() (any, error)) error {
	// 1. 先尝试获取
	err := c.Get(ctx, key, value)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrCacheMiss) {
		return err
	}

	// 2. 缓存未命中，使用 singleflight 合并回源请求
	// 确保同一个 key 只有一个协程在跑 fn
	v, err, _ := c.sfg.Do(key, func() (any, error) {
		// 双重检查：进锁后再查一次，防止刚才排队期间别人已经写好了
		var innerVal any
		if err := c.Get(ctx, key, &innerVal); err == nil {
			return innerVal, nil
		}

		// 执行真正的业务回源逻辑
		data, err := fn()
		if err != nil {
			return nil, err
		}

		// 异步回写缓存
		_ = c.Set(ctx, key, data, expiration)
		return data, nil
	})

	if err != nil {
		return err
	}

	// 将结果反序列化到传入的 value 指针
	// 注意：由于 singleflight 返回的是 any，我们需要处理类型转换
	b, _ := json.Marshal(v)
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
	n, err := c.client.Exists(ctx, c.buildKey(key)).Result()
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
