// Package limiter 提供了限流器的通用接口与多种后端实现。
package limiter

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

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
func (l *LocalLimiter) Allow(_ context.Context, _ string) (bool, error) {
	return l.limiter.Allow(), nil
}

// redisTokenBucketScript 是经过优化的 Redis Lua 脚本。
//
//nolint:gosec // 这是一个 Lua 脚本，不包含硬编码的凭据。
const redisTokenBucketScript = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = 1

local last_tokens = tonumber(redis.call("hget", key, "tokens"))
local last_refreshed = tonumber(redis.call("hget", key, "last_refreshed"))

if last_tokens == nil then
    last_tokens = burst
    last_refreshed = now
end

local delta = math.max(0, now - last_refreshed)
local new_tokens = math.min(burst, last_tokens + (delta * rate))

local allowed = false
if new_tokens >= requested then
    new_tokens = new_tokens - requested
    allowed = true
end

redis.call("hset", key, "tokens", new_tokens, "last_refreshed", now)
redis.call("expire", key, math.ceil(burst / rate) * 2)

return allowed and 1 or 0
`

// RedisLimiter 是基于 Redis + Lua 脚本实现的分布式令牌桶限流器。
type RedisLimiter struct {
	client redis.UniversalClient // Redis 客户端实例。
	rate   int                   // 每秒产生的令牌数 (QPS)。
	burst  int                   // 令牌桶的最大容量。
}

// NewRedisLimiter 创建并初始化一个分布式限流器。
func NewRedisLimiter(client redis.UniversalClient, fillingRate int, _ time.Duration) *RedisLimiter {
	// 默认突发流量容量为速率的 120%。
	const burstMultiplier = 1.2
	burstVal := int(float64(fillingRate) * burstMultiplier)

	if burstVal <= 0 {
		burstVal = 1
	}

	slog.Info("redis_limiter initialized", "rate", fillingRate, "burst", burstVal)

	return &RedisLimiter{
		client: client,
		rate:   fillingRate,
		burst:  burstVal,
	}
}

// Allow 实现 Limiter 接口。
func (l *RedisLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().Unix()

	// 执行 Lua 脚本。
	result, err := l.client.Eval(ctx, redisTokenBucketScript, []string{key}, l.rate, l.burst, now).Int()
	if err != nil {
		return false, fmt.Errorf("redis eval failed: %w", err)
	}

	return result == 1, nil
}
