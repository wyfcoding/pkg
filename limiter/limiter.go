// Package limiter 提供了限流器的通用接口与多种后端实现。
package limiter

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

//go:embed token_bucket.lua
var redisTokenBucketScript string

// Limiter 定义了限流器的通用接口。
type Limiter interface {
	// Allow 检查指定的 key 是否允许通过限流。
	Allow(ctx context.Context, key string) (bool, error)
}

// LocalLimiter 是基于内存的本地令牌桶限流器。
type LocalLimiter struct {
	limiter *rate.Limiter
}

// NewLocalLimiter 创建本地限流器。
func NewLocalLimiter(fillingRate rate.Limit, burst int) *LocalLimiter {
	slog.Info("local_limiter initialized", "rate", fillingRate, "burst", burst)

	return &LocalLimiter{
		limiter: rate.NewLimiter(fillingRate, burst),
	}
}

// Allow 实现 Limiter 接口。
func (l *LocalLimiter) Allow(ctx context.Context, key string) (bool, error) {
	// 检查 Context 是否已取消，体现对链路状态的尊重。
	if err := ctx.Err(); err != nil {
		return false, err
	}

	allowed := l.limiter.Allow()
	if !allowed {
		// 记录具体哪个 key 触发了本地限流，这对于排查热点请求非常有意义。
		slog.WarnContext(ctx, "local rate limit triggered", "key", key)
	}

	return allowed, nil
}

// RedisLimiter 是基于 Redis + Lua 脚本实现的分布式令牌桶限流器。
type RedisLimiter struct {
	client redis.UniversalClient // Redis 客户端实例。
	script *redis.Script         // Lua 脚本执行器 (自动处理 EVALSHA)。
	rate   int                   // 每秒产生的令牌数 (QPS)。
	burst  int                   // 令牌桶的最大容量。
}

// NewRedisLimiter 创建并初始化一个分布式限流器。
func NewRedisLimiter(client redis.UniversalClient, rate, burst int) *RedisLimiter {
	if burst <= 0 {
		// 默认突发流量容量为速率的 120%。
		const burstMultiplier = 1.2
		burst = int(float64(rate) * burstMultiplier)
	}

	if burst <= 0 {
		burst = 1
	}

	slog.Info("redis_limiter initialized", "rate", rate, "burst", burst)

	return &RedisLimiter{
		client: client,
		script: redis.NewScript(redisTokenBucketScript),
		rate:   rate,
		burst:  burst,
	}
}

// Allow 实现 Limiter 接口。
func (l *RedisLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().Unix()

	// 执行 Lua 脚本 (尝试 EVALSHA，失败则 EVAL)。
	result, err := l.script.Run(ctx, l.client, []string{key}, l.rate, l.burst, now).Int()
	if err != nil {
		return false, fmt.Errorf("redis script run failed: %w", err)
	}

	return result == 1, nil
}
