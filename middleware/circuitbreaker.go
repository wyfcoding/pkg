package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HttpCircuitBreaker Gin 熔断中间件
func HttpCircuitBreaker(cfg config.CircuitBreakerConfig, m *metrics.Metrics) gin.HandlerFunc {
	b := breaker.NewBreaker(breaker.Settings{
		Name:   "HTTP-Inbound",
		Config: cfg,
	}, m)

	return func(c *gin.Context) {
		_, err := b.Execute(func() (any, error) {
			c.Next()
			if c.Writer.Status() >= 500 {
				return nil, http.ErrHandlerTimeout
			}
			return nil, nil
		})

		if err != nil && err == breaker.ErrServiceUnavailable {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "circuit breaker open"})
		}
	}
}

// GrpcCircuitBreaker gRPC 熔断拦截器
func GrpcCircuitBreaker(cfg config.CircuitBreakerConfig, m *metrics.Metrics) grpc.UnaryServerInterceptor {
	b := breaker.NewBreaker(breaker.Settings{
		Name:   "GRPC-Inbound",
		Config: cfg,
	}, m)

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := b.Execute(func() (any, error) {
			return handler(ctx, req)
		})

		if err != nil && err == breaker.ErrServiceUnavailable {
			return nil, status.Error(codes.Unavailable, "circuit breaker open")
		}
		return resp, err
	}
}
