// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/response"
)

// Recovery 返回一个用于捕获 Panic 并恢复的 Gin 中间件。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 记录包含详细上下文的结构化错误日志。
				logging.Error(c.Request.Context(), "panic recovered in http request",
					"error", err,
					"stack", string(debug.Stack()),
					"path", c.Request.URL.Path,
				)

				// 返回符合项目标准的 500 内部服务器错误响应。
				response.ErrorWithStatus(c, http.StatusInternalServerError,
					"Internal Server Error",
					fmt.Sprintf("A panic occurred: %v", err))

				c.Abort()
			}
		}()

		c.Next()
	}
}