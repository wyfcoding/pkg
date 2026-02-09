// Package limiter 提供了限流器的实现。
// 增强：实现了基于 Redis ZSet 的滑动窗口限流算法，相比令牌桶能更精确地控制窗口内的请求总量。
package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// SlidingWindowLimiter 分布式滑动窗口限流器。
type SlidingWindowLimiter struct {
	client redis.UniversalClient
	window time.Duration // 窗口大小
	limit  int64         // 窗口内最大请求数
}

// NewSlidingWindowLimiter 创建新的滑动窗口限流器。
func NewSlidingWindowLimiter(client redis.UniversalClient, window time.Duration, limit int64) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		client: client,
		window: window,
		limit:  limit,
	}
}

// Allow 检查指定的 key 是否允许通过。
func (l *SlidingWindowLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now()
	nowMs := now.UnixNano() / 1e6
	windowStartMs := now.Add(-l.window).UnixNano() / 1e6

	// 使用 Redis 事务确保原子性
	pipe := l.client.TxPipeline()

	// 1. 移除窗口外的旧数据
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStartMs))

	// 2. 获取当前窗口内的请求数
	pipe.ZCard(ctx, key)

	// 3. 尝试添加当前请求
	// 注意：此处是先检查 ZCard 结果，但由于是管道，我们需要在结果返回后判断。
	// 为了真正的原子性判断，更好的是使用 LUA 脚本。

	return l.allowWithLua(ctx, key, nowMs, windowStartMs)
}

// allowWithLua 使用 LUA 脚本实现滑动窗口逻辑。
func (l *SlidingWindowLimiter) allowWithLua(ctx context.Context, key string, nowMs, startMs int64) (bool, error) {
	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		
		redis.call('ZREMRANGEBYSCORE', key, 0, start)
		local count = redis.call('ZCARD', key)
		
		if count < limit then
			redis.call('ZADD', key, now, now .. '_' .. math.random())
			redis.call('PEXPIRE', key, (now - start) * 2) -- 自动过期，两倍窗口时间以稳健
			return 1
		else
			return 0
		end
	`

	result, err := l.client.Eval(ctx, script, []string{key}, nowMs, startMs, l.limit).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}
