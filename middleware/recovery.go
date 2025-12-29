package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/wyfcoding/pkg/response"

	"github.com/gin-gonic/gin"
)

// Recovery 结构化异常恢复中间件
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				stack := string(debug.Stack())
				
				// 记录结构化日志
				logger.ErrorContext(c.Request.Context(), "Panic recovered",
					"error", err,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"query", c.Request.URL.RawQuery,
					"stack", stack,
				)

				// 返回统一的 500 错误
				response.ErrorWithStatus(c, http.StatusInternalServerError, "Internal Server Error", "An unexpected error occurred")
				c.Abort()
			}
		}()
		c.Next()
	}
}