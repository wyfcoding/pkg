// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/logging"
)

// RequestLogger 返回一个用于记录 HTTP 请求详情的 Gin 中间件。
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		logging.Info(c.Request.Context(), "http request processed",
			"method", c.Request.Method,
			"path", path,
			"query", query,
			"ip", c.ClientIP(),
			"status", status,
			"latency", latency,
			"user_agent", c.Request.UserAgent(),
		)
	}
}
