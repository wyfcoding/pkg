// Package middleware 提供了用于Gin和gRPC的通用中间件。
package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	go_redis "github.com/redis/go-redis/v9"
	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/response"
	"golang.org/x/time/rate"
)

// RateLimitMiddleware 创建一个基于给定 Limiter 接口实现的 Gin 限流中间件。
// 它适用于任何实现了 limiter.Limiter 接口的限流器（如基于本地内存或 Redis）。
func RateLimitMiddleware(l limiter.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP() // 默认使用 Client IP 作为限流 Key

		// 调用限流器的 Allow 方法检查请求是否允许通过
		allowed, err := l.Allow(c.Request.Context(), key)
		if err != nil {
			// 如果限流检查过程中发生错误（例如 Redis 连接失败），
			// 通常采取 "Fail Open" 策略，即允许请求通过，避免阻断业务，并记录错误日志。
			// 这里简单地放行，生产环境建议添加日志记录。
			c.Next()
			return
		}

		if !allowed {
			// 如果限流器拒绝请求，则返回 429 Too Many Requests
			response.ErrorWithStatus(c, http.StatusTooManyRequests, "Too Many Requests", "Rate limit exceeded")
			c.Abort()
			return
		}

		c.Next()
	}
}

// RateLimitWithLimiter 是 RateLimitMiddleware 的兼容性别名。
func RateLimitWithLimiter(l limiter.Limiter) gin.HandlerFunc {
	return RateLimitMiddleware(l)
}

// NewRedisRateLimitMiddleware 是一个便捷构造函数，用于创建基于 Redis 分布式限流的中间件。
// 适用于需要跨实例限流的场景。
// client: Redis 客户端实例。
// limit: 时间窗口内允许的最大请求数。
// window: 时间窗口大小（秒）。
func NewRedisRateLimitMiddleware(client *go_redis.Client, limit int, window int) gin.HandlerFunc {
	// 创建 Redis 分布式限流器
	redisLimiter := limiter.NewRedisLimiter(client, limit, time.Duration(window)*time.Second)
	// 返回使用该限流器的中间件
	return RateLimitMiddleware(redisLimiter)
}

// NewLocalRateLimitMiddleware 是一个便捷构造函数，用于创建基于本地内存限流的中间件。
// 适用于单实例部署或不需要严格全局限流的场景。
// limit: 每秒允许的请求数 (RPS)。
// burst: 允许的突发请求数。
func NewLocalRateLimitMiddleware(limit int, burst int) gin.HandlerFunc {
	// 创建本地令牌桶限流器
	localLimiter := limiter.NewLocalLimiter(rate.Limit(limit), burst)
	// 返回使用该限流器的中间件
	return RateLimitMiddleware(localLimiter)
}
