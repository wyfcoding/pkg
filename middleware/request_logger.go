package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

// Logger 生产级访问日志中间件
func Logger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		cost := time.Since(start)

		// 提取链路追踪 ID
		spanCtx := trace.SpanContextFromContext(c.Request.Context())
		traceID := ""
		if spanCtx.HasTraceID() {
			traceID = spanCtx.TraceID().String()
		}

		// 输出结构化日志
		logger.InfoContext(c.Request.Context(), "HTTP Request",
			"trace_id", traceID,
			"status", c.Writer.Status(),
			"method", c.Request.Method,
			"path", path,
			"query", query,
			"ip", c.ClientIP(),
			"cost", cost,
			"user_agent", c.Request.UserAgent(),
		)
	}
}
