// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
// 生成摘要:
// 1) 新增 HTTP 请求体大小限制中间件。
// 2) 同时校验 Content-Length 与实际读取流，避免超大请求拖垮服务。
// 假设:
// 1) 未配置限制时不生效。
package middleware

import (
	"net/http"

	"github.com/wyfcoding/pkg/response"

	"github.com/gin-gonic/gin"
)

// MaxBodyBytes 返回一个限制请求体大小的 Gin 中间件。
func MaxBodyBytes(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limit <= 0 {
			c.Next()
			return
		}

		if c.Request.ContentLength > limit && c.Request.ContentLength != -1 {
			response.ErrorWithStatus(c, http.StatusRequestEntityTooLarge, "request body too large", "content length exceeded")
			c.Abort()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
