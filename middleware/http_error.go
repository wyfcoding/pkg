// Package middleware 提供了通用的 Gin 与 gRPC 中间件实现。
// 生成摘要:
// 1) 新增 HTTP 错误处理器，自动统一错误响应输出。
// 假设:
// 1) 业务层通过 c.Error 或返回 xerrors 来标注错误。
package middleware

import (
	"github.com/wyfcoding/pkg/response"

	"github.com/gin-gonic/gin"
)

// HTTPErrorHandler 返回一个 Gin 中间件，用于统一处理业务错误。
func HTTPErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if c.Writer.Written() {
			return
		}

		if len(c.Errors) == 0 {
			return
		}

		last := c.Errors.Last()
		if last == nil {
			return
		}

		response.Error(c, last.Err)
	}
}
