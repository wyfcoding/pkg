// Package middleware 提供了用于Gin和gRPC的通用中间件。
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	go_redis "github.com/redis/go-redis/v9"
	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/response"
	"golang.org/x/time/rate"
)

// RateLimitMiddleware 构造一个通用的 Gin 限流中间件。
// 默认策略：使用客户端 IP 作为限流标识。
func RateLimitMiddleware(l limiter.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()

		allowed, err := l.Allow(c.Request.Context(), key)
		if err != nil {
			// 遵循 Fail-Open 策略：限流组件故障时不阻断业务，但必须记录告警日志。
			slog.ErrorContext(c.Request.Context(), "rate limiter internal error, fail-open applied", "key", key, "error", err)
			c.Next()
			return
		}

		if !allowed {
			// 限流触发，记录审计日志
			slog.WarnContext(c.Request.Context(), "request rejected by rate limiter", "key", key, "path", c.Request.URL.Path)
			response.ErrorWithStatus(c, http.StatusTooManyRequests, "too many requests", "access rate limit exceeded")
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
