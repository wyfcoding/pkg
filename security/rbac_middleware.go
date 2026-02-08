package security

import (
	"net/http"
	"slices"
	"strings"

	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/response"

	"github.com/gin-gonic/gin"
)

// RBACMiddleware 检查用户角色是否满足路径要求
func RBACMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(allowedRoles) == 0 {
			c.Next()
			return
		}

		roles := extractRoles(c)
		if len(roles) == 0 {
			response.ErrorWithStatus(c, http.StatusUnauthorized, "user role not found", "")
			c.Abort()
			return
		}

		normalizedAllowed := normalizeRoles(allowedRoles)
		for _, role := range roles {
			upper := strings.ToUpper(role)
			if upper == "ADMIN" || slices.Contains(normalizedAllowed, upper) {
				c.Next()
				return
			}
		}

		response.ErrorWithStatus(c, http.StatusForbidden, "insufficient permissions", "")
		c.Abort()
	}
}

func extractRoles(c *gin.Context) []string {
	if c == nil || c.Request == nil {
		return nil
	}

	ctx := c.Request.Context()
	roles := make([]string, 0, 2)
	if role := contextx.GetRole(ctx); role != "" {
		roles = append(roles, role)
	}

	if val, ok := c.Get("roles"); ok {
		switch typed := val.(type) {
		case []string:
			roles = append(roles, typed...)
		case string:
			roles = append(roles, typed)
		}
	}

	return roles
}

func normalizeRoles(roles []string) []string {
	normalized := make([]string, 0, len(roles))
	for _, role := range roles {
		if role == "" {
			continue
		}
		normalized = append(normalized, strings.ToUpper(role))
	}
	return normalized
}
