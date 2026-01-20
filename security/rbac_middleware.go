package security

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RBACMiddleware 检查用户角色是否满足路径要求
func RBACMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 假设用户信息已经由 Auth 中间件注入到 Context 或 Header
		// 此处演示从 Context 获取角色
		role, exists := c.Get("user_role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "user role not found"})
			return
		}

		userRole := role.(string)
		isAllowed := false
		for _, r := range allowedRoles {
			if userRole == r {
				isAllowed = true
				break
			}
		}

		if !isAllowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		c.Next()
	}
}
