// Package middleware 提供了通用的 Gin 中间件。
// 生成摘要:
// 1) 访问日志字段统一使用 duration 作为耗时键。
// 假设:
// 1) 上游日志处理按 duration 读取耗时字段。
package middleware

import (
	"time"

	"github.com/wyfcoding/pkg/logging"

	"github.com/gin-gonic/gin"
)

// RequestLogger 返回一个用于记录 HTTP 请求详情的 Gin 中间件。
// 优化：支持排除路径（如健康检查）以减少无效日志，并针对慢请求和错误请求自动提升关注度。
func RequestLogger(skipPaths ...string) gin.HandlerFunc {
	skip := make(map[string]struct{})
	for _, path := range skipPaths {
		skip[path] = struct{}{}
	}

	const slowThreshold = 500 * time.Millisecond

	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		// 跳过不需要记录的路径
		if _, ok := skip[path]; ok {
			c.Next()
			return
		}

		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []any{
			"method", c.Request.Method,
			"path", path,
			"query", query,
			"ip", c.ClientIP(),
			"status", status,
			"duration", latency,
			"user_agent", c.Request.UserAgent(),
		}

		// 动态决定日志级别
		switch {
		case status >= 500:
			logging.Error(c.Request.Context(), "http request error", fields...)
		case status >= 400:
			logging.Warn(c.Request.Context(), "http request client error", fields...)
		case latency > slowThreshold:
			logging.Warn(c.Request.Context(), "http request slow query", fields...)
		default:
			logging.Info(c.Request.Context(), "http request processed", fields...)
		}
	}
}
