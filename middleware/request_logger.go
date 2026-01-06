package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

// Logger 提供生产级的 HTTP 访问日志记录功能，集成了分布式追踪信息。
// 遵循项目规范：输出包含 trace_id, span_id, duration 等核心字段的结构化日志。
func Logger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		duration := time.Since(start)

		// 从上下文中提取 OpenTelemetry 追踪信息
		spanCtx := trace.SpanContextFromContext(c.Request.Context())
		var traceID, spanID string
		if spanCtx.IsValid() {
			traceID = spanCtx.TraceID().String()
			spanID = spanCtx.SpanID().String()
		}

		// 输出标准结构化日志
		logger.InfoContext(c.Request.Context(), "http request",
			"trace_id", traceID,
			"span_id", spanID,
			"status", c.Writer.Status(),
			"method", c.Request.Method,
			"path", path,
			"query", query,
			"ip", c.ClientIP(),
			"duration", duration,
			"user_agent", c.Request.UserAgent(),
		)
	}
}
