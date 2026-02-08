// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/response"
	"google.golang.org/grpc"
)

// TimeoutMiddleware 设置请求的上下文超时保护。
func TimeoutMiddleware(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if duration <= 0 {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), duration)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)

		c.Next()

		if ctx.Err() == context.DeadlineExceeded && !c.Writer.Written() {
			response.ErrorWithStatus(c, http.StatusGatewayTimeout, "Request Timeout", "")
			c.Abort()
		}
	}
}

// GRPCTimeoutInterceptor 为 gRPC 提供一元调用的超时拦截器。
func GRPCTimeoutInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if timeout <= 0 {
			return handler(ctx, req)
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return handler(ctx, req)
	}
}
