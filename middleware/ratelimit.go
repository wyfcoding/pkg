// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现.
package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/response"
	"golang.org/x/time/rate"
)

// RateLimitWithLimiter 返回一个使用指定限流器的 Gin 中间件.
func RateLimitWithLimiter(l limiter.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP() + ":" + c.Request.URL.Path
		allowed, err := l.Allow(c.Request.Context(), key)
		if err != nil {
			logging.Error(c.Request.Context(), "rate limit check failed", "key", key, "error", err)
			c.Next()

			return
		}

		if !allowed {
			logging.Warn(c.Request.Context(), "rate limit exceeded", "key", key)
			response.ErrorWithStatus(c, http.StatusTooManyRequests, "Rate Limit Exceeded", "")
			c.Abort()

			return
		}

		c.Next()
	}
}

// NewDistributedRateLimitMiddleware 创建 Redis 分布式限流中间件.
func NewDistributedRateLimitMiddleware(client *redis.Client, rateLimit, _ int) gin.HandlerFunc {
	l := limiter.NewRedisLimiter(client, rateLimit, time.Second)

	return RateLimitWithLimiter(l)
}

// NewLocalRateLimitMiddleware 创建本地令牌桶限流中间件.
func NewLocalRateLimitMiddleware(rateLimit, burst int) gin.HandlerFunc {
	l := limiter.NewLocalLimiter(rate.Limit(rateLimit), burst)

	return RateLimitWithLimiter(l)
}
