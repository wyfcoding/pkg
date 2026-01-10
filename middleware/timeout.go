// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/response"
)

// TimeoutMiddleware 设置请求的强制硬超时保护。
func TimeoutMiddleware(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), duration)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)

		done := make(chan struct{}, 1)

		go func() {
			c.Next()
			done <- struct{}{}
		}()

		select {
		case <-done:
			return
		case <-ctx.Done():
			response.ErrorWithStatus(c, http.StatusGatewayTimeout, "Request Timeout", "")
			c.Abort()
		}
	}
}