package middleware

import (
	"net/http"
	"strings"

	"github.com/wyfcoding/pkg/jwt"
	"github.com/wyfcoding/pkg/response"

	"github.com/gin-gonic/gin"
)

// JWTAuth 增强版：支持基础认证并注入用户信息
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.ErrorWithStatus(c, http.StatusUnauthorized, "missing authorization header", "")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.ErrorWithStatus(c, http.StatusUnauthorized, "invalid authorization format", "")
			c.Abort()
			return
		}

		claims, err := jwt.ParseToken(parts[1], secret)
		if err != nil {
			response.ErrorWithStatus(c, http.StatusUnauthorized, "invalid or expired token", "")
			c.Abort()
			return
		}

		// 注入上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("roles", claims.Roles) // 假设 JWT 中包含角色列表
		c.Next()
	}
}

// HasRole 提供角色权限校验中间件。
// 只有当 JWT 中的角色列表包含指定 role 或拥有超级管理员权限 (ADMIN) 时，才允许通过。
func HasRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles, exists := c.Get("roles")
		if !exists {
			response.ErrorWithStatus(c, http.StatusForbidden, "Forbidden", "no roles assigned")
			c.Abort()
			return
		}

		// 遍历用户角色列表进行鉴权
		for _, r := range roles.([]string) {
			if r == role || r == "ADMIN" { // ADMIN 角色默认为超级管理员，具备所有权限
				c.Next()
				return
			}
		}

		response.ErrorWithStatus(c, http.StatusForbidden, "Forbidden", "insufficient role permissions")
		c.Abort()
	}
}

// GetUserID 是一个安全的 Context 提取函数，用于从 Gin 请求上下文中获取已认证的用户 ID。
// 支持多种底层数据类型的健壮转换，并返回提取是否成功的布尔值。
func GetUserID(c *gin.Context) (uint64, bool) {
	val, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	// 执行类型断言转换
	switch v := val.(type) {
	case uint64:
		return v, true
	case float64: // 针对部分 JSON 解析库可能将 ID 识别为 float64 的兼容性处理
		return uint64(v), true
	default:
		return 0, false
	}
}
