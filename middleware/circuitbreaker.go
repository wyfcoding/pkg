// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HTTPCircuitBreaker 构造一个应用于 Gin 框架的入站请求熔断中间件。
func HTTPCircuitBreaker(cfg config.CircuitBreakerConfig, m *metrics.Metrics) gin.HandlerFunc {
	circuitBreaker := breaker.NewBreaker(breaker.Settings{
		Name:   "HTTP-Inbound",
		Config: cfg,
	}, m)

	return func(c *gin.Context) {
		_, err := circuitBreaker.Execute(func() (any, error) {
			c.Next()

			const failureStatusCodeThreshold = 500
			if c.Writer.Status() >= failureStatusCodeThreshold {
				return nil, http.ErrHandlerTimeout
			}

			return nil, nil
		})

		if err != nil && errors.Is(err, breaker.ErrServiceUnavailable) {
			slog.WarnContext(c.Request.Context(), "http request rejected by inbound circuit breaker", "path", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "circuit breaker open"})
		}
	}
}

// GRPCCircuitBreaker 构造一个应用于 gRPC Server 的入站请求熔断拦截器。
func GRPCCircuitBreaker(cfg config.CircuitBreakerConfig, m *metrics.Metrics) grpc.UnaryServerInterceptor {
	circuitBreaker := breaker.NewBreaker(breaker.Settings{
		Name:   "GRPC-Inbound",
		Config: cfg,
	}, m)

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := circuitBreaker.Execute(func() (any, error) {
			return handler(ctx, req)
		})

		if err != nil && errors.Is(err, breaker.ErrServiceUnavailable) {
			slog.WarnContext(ctx, "grpc call rejected by inbound circuit breaker", "method", info.FullMethod)

			return nil, status.Error(codes.Unavailable, "circuit breaker open")
		}

		return resp, err
	}
}
