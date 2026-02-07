// Package middleware 提供了通用的 Gin 与 gRPC 中间件实现。
// 生成摘要:
// 1) 新增 HTTP 上下文增强中间件，自动注入 IP/UA/租户/用户/权限/角色信息。
// 2) 通过请求头统一透传关键身份字段。
// 假设:
// 1) 租户 ID 使用请求头 X-Tenant-ID 传递。
// 2) 用户 ID 使用请求头 X-User-ID 传递，权限范围使用 X-Scopes。
package middleware

import (
	"github.com/wyfcoding/pkg/contextx"

	"github.com/gin-gonic/gin"
)

// HeaderXTenantID 定义租户 ID 的请求头名称。
const HeaderXTenantID = "X-Tenant-ID"

// HeaderXUserID 定义用户 ID 的请求头名称。
const HeaderXUserID = "X-User-ID"

// HeaderXScopes 定义权限范围的请求头名称。
const HeaderXScopes = "X-Scopes"

// HeaderXRole 定义角色的请求头名称。
const HeaderXRole = "X-Role"

// RequestContextEnricher 返回一个 Gin 中间件，用于注入常用上下文字段。
func RequestContextEnricher() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = contextx.WithIP(ctx, c.ClientIP())
		ctx = contextx.WithUserAgent(ctx, c.Request.UserAgent())

		if tenantID := c.GetHeader(HeaderXTenantID); tenantID != "" {
			ctx = contextx.WithTenantID(ctx, tenantID)
		}
		if userID := c.GetHeader(HeaderXUserID); userID != "" {
			ctx = contextx.WithUserID(ctx, userID)
		}
		if scopes := c.GetHeader(HeaderXScopes); scopes != "" {
			ctx = contextx.WithScopes(ctx, scopes)
		}
		if role := c.GetHeader(HeaderXRole); role != "" {
			ctx = contextx.WithRole(ctx, role)
		}

		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
