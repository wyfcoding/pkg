// Package middleware 提供了通用的 Gin 与 gRPC 中间件实现。
// 生成摘要:
// 1) 新增 HTTP 上下文增强中间件，自动注入 IP/UA/租户信息。
// 假设:
// 1) 租户 ID 使用请求头 X-Tenant-ID 传递。
package middleware

import (
	"github.com/wyfcoding/pkg/contextx"

	"github.com/gin-gonic/gin"
)

// HeaderXTenantID 定义租户 ID 的请求头名称。
const HeaderXTenantID = "X-Tenant-ID"

// RequestContextEnricher 返回一个 Gin 中间件，用于注入常用上下文字段。
func RequestContextEnricher() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = contextx.WithIP(ctx, c.ClientIP())
		ctx = contextx.WithUserAgent(ctx, c.Request.UserAgent())

		if tenantID := c.GetHeader(HeaderXTenantID); tenantID != "" {
			ctx = contextx.WithTenantID(ctx, tenantID)
		}

		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
