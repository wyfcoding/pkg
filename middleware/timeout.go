package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/response"
)

// TimeoutMiddleware 设置请求的强制硬超时
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 创建一个带超时的 Context
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		// 更新 Gin 请求的 Context
		c.Request = c.Request.WithContext(ctx)

		// 使用 channel 等待处理结果
		finished := make(chan struct{}, 1)
		go func() {
			c.Next()
			finished <- struct{}{}
		}()

		select {
		case <-finished:
			return
		case <-ctx.Done():
			// 如果 Context 先结束，说明超时了
			if ctx.Err() == context.DeadlineExceeded {
				response.ErrorWithStatus(c, http.StatusGatewayTimeout, "Gateway Timeout", "Request took too long to process")
				c.Abort()
			}
		}
	}
}
