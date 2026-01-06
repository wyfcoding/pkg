package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/response"
	"go.opentelemetry.io/otel/trace"
)

// Recovery 提供结构化的异常恢复机制，捕获处理流程中的 Panic 并转换为标准错误响应。
// 遵循项目规范：记录包含堆栈信息和分布式追踪 ID 的错误日志。
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				stack := string(debug.Stack())

				// 提取追踪信息以便于在日志平台快速定位崩溃位置
				spanCtx := trace.SpanContextFromContext(c.Request.Context())
				var traceID string
				if spanCtx.IsValid() {
					traceID = spanCtx.TraceID().String()
				}

				// 记录包含详细上下文的结构化错误日志
				logger.ErrorContext(c.Request.Context(), "panic recovered",
					"error", err,
					"trace_id", traceID,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"stack", stack,
				)

				// 返回符合项目标准的 500 内部服务器错误响应
				response.ErrorWithStatus(c, http.StatusInternalServerError, "internal server error", "An unexpected system error occurred")
				c.Abort()
			}
		}()
		c.Next()
	}
}
