package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9" // 导入Redis客户端库。
	"golang.org/x/time/rate"       // 导入基于令牌桶算法的限流库。
)

// Limiter 接口定义了限流器的通用行为。
// 任何实现了此接口的类型都可以用作限流器。
type Limiter interface {
	Allow(ctx context.Context, key string) (bool, error) // 检查是否允许请求通过。
}

// LocalLimiter 是一个基于令牌桶算法的本地限流器。
// 它适用于单个应用程序实例内的限流。
type LocalLimiter struct {
	limiter *rate.Limiter // 底层的令牌桶限流器实例。
}

// NewLocalLimiter 创建并返回一个新的 LocalLimiter 实例。
// r: 每秒生成的令牌数，代表允许的平均请求速率。
// b: 令牌桶的容量，代表允许的瞬时突发请求数。
func NewLocalLimiter(r rate.Limit, b int) *LocalLimiter {
	return &LocalLimiter{
		limiter: rate.NewLimiter(r, b),
	}
}

// Allow 检查一个请求是否被 LocalLimiter 允许通过。
// key: 请求的唯一标识符（例如，用户ID或IP地址），但在此本地限流器中，key参数未被使用，因为它是全局限流。
// 返回值：一个布尔值，表示是否允许请求；一个错误，如果发生内部错误。
func (l *LocalLimiter) Allow(ctx context.Context, key string) (bool, error) {
	// Allow() 方法会尝试从令牌桶中获取一个令牌。
	// 如果令牌可用，则立即返回 true；否则，如果桶已空，则返回 false。
	return l.limiter.Allow(), nil
}

// RedisLimiter 是一个基于 Redis 实现的分布式限流器。
// 它使用Redis的ZSet（有序集合）数据结构实现滑动窗口算法，支持在多个应用程序实例之间共享限流状态。
type RedisLimiter struct {
	client *redis.Client // Redis客户端实例。
	limit  int           // 在指定时间窗口内允许的最大请求数。
	window time.Duration // 时间窗口的长度。
}

// NewRedisLimiter 创建并返回一个新的 RedisLimiter 实例。
// client: 已连接的Redis客户端实例。
// limit: 时间窗口内允许的最大请求数。
// window: 时间窗口的持续时间。
func NewRedisLimiter(client *redis.Client, limit int, window time.Duration) *RedisLimiter {
	return &RedisLimiter{
		client: client,
		limit:  limit,
		window: window,
	}
}

// Allow 检查一个请求是否被 RedisLimiter 允许通过。
// 它实现了基于Redis的滑动窗口算法：
// 1. 移除时间窗口之外的旧请求记录。
// 2. 统计当前时间窗口内的请求数量。
// 3. 如果请求数量未超过限制，则记录当前请求并允许通过。
// key: 请求的唯一标识符，通常用作Redis键（例如，"rate_limit:user:123"）。
// 返回值：一个布尔值，表示是否允许请求；一个错误，如果发生Redis操作错误。
func (l *RedisLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().UnixNano()                // 当前时间（纳秒）。
	windowStart := now - l.window.Nanoseconds() // 时间窗口的起始时间。

	// 使用Redis Pipeline批量执行命令，确保原子性。
	pipe := l.client.Pipeline()

	// 步骤1: 移除时间窗口之外（早于 windowStart）的旧请求记录。
	// ZRemRangeByScore 根据分数范围移除有序集合的成员。
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))

	// 步骤2: 统计当前时间窗口内剩余的请求数。
	// ZCard 返回有序集合的成员数量。
	pipe.ZCard(ctx, key)

	// 步骤3: 添加当前请求的记录到有序集合中，分数为当前时间戳。
	// ZAdd 将成员添加到有序集合中，并指定分数（时间戳）。
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now), // 使用时间戳作为分数。
		Member: now,          // 成员可以是唯一标识符，这里也使用了时间戳。
	})

	// 步骤4: 设置Redis键的过期时间，防止内存泄露。
	pipe.Expire(ctx, key, l.window)

	// 执行所有Pipeline中的命令。
	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	// 获取ZCard命令的执行结果，即当前窗口内的请求数。
	// cmds[1] 是 ZCard 命令的结果，cmds[0] 是 ZRemRangeByScore 的结果。
	count := cmds[1].(*redis.IntCmd).Val()

	// 判断是否允许请求通过：如果当前窗口内的请求数小于限制，则允许。
	// 注意：ZAdd 操作也会增加一个请求，所以这里的 count 包含了当前请求。
	return count < int64(l.limit), nil
}
