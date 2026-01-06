package middleware

import (
	"context"
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

// HttpCircuitBreaker 构造一个应用于 Gin 框架的入站请求熔断中间件。
// 它会自动识别 HTTP 5xx 状态码作为失败采样点，并在达到阈值时开启熔断。
func HttpCircuitBreaker(cfg config.CircuitBreakerConfig, m *metrics.Metrics) gin.HandlerFunc {
	b := breaker.NewBreaker(breaker.Settings{
		Name:   "HTTP-Inbound",
		Config: cfg,
	}, m)

	return func(c *gin.Context) {
		_, err := b.Execute(func() (any, error) {
			c.Next()
			// 如果下游业务返回 500+ 错误，判定为服务故障，触发熔断采样
			if c.Writer.Status() >= 500 {
				return nil, http.ErrHandlerTimeout
			}
			return nil, nil
		})

		if err != nil && err == breaker.ErrServiceUnavailable {
			slog.WarnContext(c.Request.Context(), "http request rejected by inbound circuit breaker", "path", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "circuit breaker open"})
		}
	}
}

// GrpcCircuitBreaker 构造一个应用于 gRPC Server 的入站请求熔断拦截器。
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
			slog.WarnContext(ctx, "grpc call rejected by inbound circuit breaker", "method", info.FullMethod)
			return nil, status.Error(codes.Unavailable, "circuit breaker open")
		}
		return resp, err
	}
}
