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

// HasRole 角色校验中间件
func HasRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roles, exists := c.Get("roles")
		if !exists {
			response.ErrorWithStatus(c, http.StatusForbidden, "Forbidden", "no roles assigned")
			c.Abort()
			return
		}

		for _, r := range roles.([]string) {
			if r == role || r == "ADMIN" { // 管理员拥有所有权限
				c.Next()
				return
			}
		}

		response.ErrorWithStatus(c, http.StatusForbidden, "Forbidden", "insufficient role permissions")
		c.Abort()
	}
}

// --- Getter 辅助函数 (保持向后兼容) ---

func GetUserID(c *gin.Context) (uint64, bool) {
	val, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	// 支持多种数值类型断言，增强鲁棒性
	switch v := val.(type) {
	case uint64:
		return v, true
	case float64:
		return uint64(v), true
	default:
		return 0, false
	}
}

func MustGetUserID(c *gin.Context) uint64 {
	id, ok := GetUserID(c)
	if !ok {
		panic("user_id not found in context")
	}
	return id
}