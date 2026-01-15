// Package middleware 提供了通用的中间件实现.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/idgen"
)

const (
	HeaderXRequestID = "X-Request-ID"
)

// RequestID 返回一个用于生成或传递请求 ID 的 Gin 中间件。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 尝试从 Header 获取已有的 Request ID
		requestID := c.GetHeader(HeaderXRequestID)
		if requestID == "" {
			// 2. 如果没有，则生成一个新的分布式唯一 ID (雪花算法)
			requestID = idgen.GenIDString()
		}

		// 3. 注入到 Context 中供后续业务使用
		c.Request = c.Request.WithContext(contextx.WithRequestID(c.Request.Context(), requestID))

		// 4. 设置到 Response Header 中，方便客户端追踪
		c.Header(HeaderXRequestID, requestID)

		c.Next()
	}
}
