package limiter

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// Limiter 定义了限流器的通用接口。
type Limiter interface {
	// Allow 检查指定的 key 是否允许通过限流。
	Allow(ctx context.Context, key string) (bool, error)
}

// --- Local Limiter (Token Bucket) ---

// LocalLimiter 是基于内存的本地令牌桶限流器。
type LocalLimiter struct {
	limiter *rate.Limiter
}

// NewLocalLimiter 创建本地限流器。
// r: 每秒填充速率，b: 桶容量。
func NewLocalLimiter(r rate.Limit, b int) *LocalLimiter {
	return &LocalLimiter{
		limiter: rate.NewLimiter(r, b),
	}
}

func (l *LocalLimiter) Allow(ctx context.Context, key string) (bool, error) {
	return l.limiter.Allow(), nil
}

// redisTokenBucketScript 是经过充分优化的 Redis Lua 脚本。
// 该脚本实现了标准的令牌桶算法，具有以下特点：
// 1. 原子性：所有计算都在 Redis 服务端完成。
// 2. 高性能：仅需存储两个 Hash 字段，内存开销极小。
// 3. 实时性：基于时间戳动态计算令牌增量。
const redisTokenBucketScript = `
local key = KEYS[1]           -- 限流标识
local rate = tonumber(ARGV[1]) -- 每秒生成令牌速率
local burst = tonumber(ARGV[2])-- 桶最大容量
local now = tonumber(ARGV[3])  -- 当前时间戳 (秒)
local requested = 1            -- 请求令牌数 (默认为1)

-- 获取当前状态
local last_tokens = tonumber(redis.call("hget", key, "tokens"))
local last_refreshed = tonumber(redis.call("hget", key, "last_refreshed"))

-- 第一次初始化
if last_tokens == nil then
    last_tokens = burst
    last_refreshed = now
end

-- 计算自上次刷新以来新生成的令牌
local delta = math.max(0, now - last_refreshed)
local new_tokens = math.min(burst, last_tokens + (delta * rate))

-- 检查令牌是否足够
local allowed = false
if new_tokens >= requested then
    new_tokens = new_tokens - requested
    allowed = true
end

-- 更新 Redis 状态并设置过期时间（防止内存泄漏，过期时间设为桶填满所需时间的2倍）
redis.call("hset", key, "tokens", new_tokens, "last_refreshed", now)
redis.call("expire", key, math.ceil(burst / rate) * 2)

return allowed and 1 or 0
`

// RedisLimiter 是基于 Redis 和 Lua 脚本实现的分布式令牌桶限流器。
type RedisLimiter struct {
	client *redis.Client
	rate   int // 每秒产生的令牌数
	burst  int // 桶的最大容量
}

// NewRedisLimiter 创建一个新的分布式限流器。
// client: Redis 客户端。
// rate: 每秒限制的请求数。
// window: 在分布式令牌桶中，我们简化为每秒速率，如果传入 1s 则 rate 即为 QPS。
func NewRedisLimiter(client *redis.Client, rate int, _ time.Duration) *RedisLimiter {
	// 默认突发流量容量为速率的 20% 或最小为 1，确保平滑
	burst := rate + (rate / 5)
	if burst <= 0 {
		burst = 1
	}
	return &RedisLimiter{
		client: client,
		rate:   rate,
		burst:  burst,
	}
}

// NewRedisLimiterWithBurst 创建带自定义突发容量的分布式限流器。
func NewRedisLimiterWithBurst(client *redis.Client, rate int, burst int) *RedisLimiter {
	return &RedisLimiter{
		client: client,
		rate:   rate,
		burst:  burst,
	}
}

// Allow 实现 Limiter 接口。
func (l *RedisLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().Unix()

	// 执行 Lua 脚本
	result, err := l.client.Eval(ctx, redisTokenBucketScript, []string{key}, l.rate, l.burst, now).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}
