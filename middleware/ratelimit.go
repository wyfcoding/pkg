// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现.
// 生成摘要:
// 1) 增加 gRPC 限流拦截器，支持本地与 Redis 分布式模式。
// 2) 限流 key 优先使用租户/用户上下文，提升隔离性。
// 假设:
// 1) 缺少租户/用户信息时，退化为 client IP + path 作为限流 key。
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// RateLimitWithLimiter 返回一个使用指定限流器的 Gin 中间件.
func RateLimitWithLimiter(l limiter.Limiter) gin.HandlerFunc {
	return RateLimitWithLimiterAndMetrics(l, nil)
}

// RateLimitWithLimiterAndMetrics 返回一个使用指定限流器与指标的 Gin 中间件.
func RateLimitWithLimiterAndMetrics(l limiter.Limiter, m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := buildLimitKey(c.Request.Context(), c.ClientIP(), c.Request.URL.Path)
		allowed, err := l.Allow(c.Request.Context(), key)
		if err != nil {
			logging.Error(c.Request.Context(), "rate limit check failed", "key", key, "error", err)
			c.Next()

			return
		}

		if !allowed {
			if m != nil && m.HTTPRateLimitTotal != nil {
				path := c.FullPath()
				if path == "" {
					path = c.Request.URL.Path
				}
				m.HTTPRateLimitTotal.WithLabelValues(c.Request.Method, path).Inc()
			}
			logging.Warn(c.Request.Context(), "rate limit exceeded", "key", key)
			response.ErrorWithStatus(c, http.StatusTooManyRequests, "Rate Limit Exceeded", "")
			c.Abort()

			return
		}

		c.Next()
	}
}

// NewDistributedRateLimitMiddleware 创建 Redis 分布式限流中间件.
func NewDistributedRateLimitMiddleware(client *redis.Client, rateLimit, burst int) gin.HandlerFunc {
	l := limiter.NewRedisLimiter(client, rateLimit, burst)

	return RateLimitWithLimiter(l)
}

// NewDistributedRateLimitMiddlewareWithMetrics 创建带指标的 Redis 分布式限流中间件.
func NewDistributedRateLimitMiddlewareWithMetrics(client *redis.Client, rateLimit, burst int, m *metrics.Metrics) gin.HandlerFunc {
	l := limiter.NewRedisLimiter(client, rateLimit, burst)

	return RateLimitWithLimiterAndMetrics(l, m)
}

// NewLocalRateLimitMiddleware 创建本地令牌桶限流中间件.
func NewLocalRateLimitMiddleware(rateLimit, burst int) gin.HandlerFunc {
	l := limiter.NewLocalLimiter(rate.Limit(rateLimit), burst)

	return RateLimitWithLimiter(l)
}

// NewLocalRateLimitMiddlewareWithMetrics 创建带指标的本地限流中间件.
func NewLocalRateLimitMiddlewareWithMetrics(rateLimit, burst int, m *metrics.Metrics) gin.HandlerFunc {
	l := limiter.NewLocalLimiter(rate.Limit(rateLimit), burst)

	return RateLimitWithLimiterAndMetrics(l, m)
}

// GRPCRateLimitWithLimiter 返回一个使用指定限流器的 gRPC 一元拦截器。
func GRPCRateLimitWithLimiter(l limiter.Limiter) grpc.UnaryServerInterceptor {
	return GRPCRateLimitWithLimiterAndMetrics(l, nil)
}

// GRPCRateLimitWithLimiterAndMetrics 返回一个使用指定限流器与指标的 gRPC 拦截器。
func GRPCRateLimitWithLimiterAndMetrics(l limiter.Limiter, m *metrics.Metrics) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		key := info.FullMethod
		if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
			key = buildLimitKey(ctx, p.Addr.String(), info.FullMethod)
		} else {
			key = buildLimitKey(ctx, "", info.FullMethod)
		}

		allowed, err := l.Allow(ctx, key)
		if err != nil {
			logging.Error(ctx, "grpc rate limit check failed", "key", key, "error", err)
			return handler(ctx, req)
		}

		if !allowed {
			if m != nil && m.GRPCRateLimitTotal != nil {
				service, method := splitFullMethod(info.FullMethod)
				m.GRPCRateLimitTotal.WithLabelValues(service, method).Inc()
			}
			logging.Warn(ctx, "grpc rate limit exceeded", "key", key)
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}

		return handler(ctx, req)
	}
}

// NewGRPCDistributedRateLimitInterceptor 创建 Redis 分布式限流拦截器。
func NewGRPCDistributedRateLimitInterceptor(client *redis.Client, rateLimit, burst int) grpc.UnaryServerInterceptor {
	l := limiter.NewRedisLimiter(client, rateLimit, burst)

	return GRPCRateLimitWithLimiter(l)
}

// NewGRPCDistributedRateLimitInterceptorWithMetrics 创建带指标的 Redis 分布式限流拦截器。
func NewGRPCDistributedRateLimitInterceptorWithMetrics(client *redis.Client, rateLimit, burst int, m *metrics.Metrics) grpc.UnaryServerInterceptor {
	l := limiter.NewRedisLimiter(client, rateLimit, burst)

	return GRPCRateLimitWithLimiterAndMetrics(l, m)
}

// NewGRPCLocalRateLimitInterceptor 创建本地令牌桶限流拦截器。
func NewGRPCLocalRateLimitInterceptor(rateLimit, burst int) grpc.UnaryServerInterceptor {
	l := limiter.NewLocalLimiter(rate.Limit(rateLimit), burst)

	return GRPCRateLimitWithLimiter(l)
}

// NewGRPCLocalRateLimitInterceptorWithMetrics 创建带指标的本地限流拦截器。
func NewGRPCLocalRateLimitInterceptorWithMetrics(rateLimit, burst int, m *metrics.Metrics) grpc.UnaryServerInterceptor {
	l := limiter.NewLocalLimiter(rate.Limit(rateLimit), burst)

	return GRPCRateLimitWithLimiterAndMetrics(l, m)
}

func buildLimitKey(ctx context.Context, addr, path string) string {
	tenantID := contextx.GetTenantID(ctx)
	userID := contextx.GetUserID(ctx)

	if tenantID == "" && userID == "" {
		if addr != "" {
			return fmt.Sprintf("%s:%s", addr, path)
		}
		return path
	}

	if addr == "" {
		addr = "unknown"
	}

	return fmt.Sprintf("%s:%s:%s:%s", tenantID, userID, addr, path)
}

func splitFullMethod(fullMethod string) (string, string) {
	if fullMethod == "" {
		return "unknown", "unknown"
	}
	fullMethod = strings.TrimPrefix(fullMethod, "/")
	parts := strings.Split(fullMethod, "/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "unknown", fullMethod
}
