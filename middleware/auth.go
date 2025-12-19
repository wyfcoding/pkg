package middleware

import (
	"net/http"
	"strings"

	"github.com/wyfcoding/pkg/jwt" // 导入JWT处理包。

	"github.com/gin-gonic/gin" // 导入Gin Web框架。
)

// JWTAuth 是一个Gin中间件，用于实现JWT（JSON Web Token）认证。
// 它会从请求头中解析JWT，验证其有效性，并将用户信息（如UserID、Username）存储到Gin的上下文中。
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从请求头中获取 Authorization 字段。
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort() // 中止请求链，不再执行后续处理器。
			return
		}

		// 解析Bearer Token格式的Authorization头。
		// 例如："Bearer <token>"。
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			c.Abort()
			return
		}

		// 提取Token字符串。
		tokenString := parts[1]
		// 解析并验证Token。
		claims, err := jwt.ParseToken(tokenString, secret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		// 将验证通过的用户信息存储到Gin的上下文中，以便后续的处理器可以访问。
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		// 继续执行请求链中的下一个中间件或处理器。
		c.Next()
	}
}

// GetUserID 从Gin上下文中获取用户ID。
// 返回用户ID和布尔值，表示是否成功获取。
func GetUserID(c *gin.Context) (uint64, bool) {
	// 从上下文中获取 "user_id" 键对应的值。
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	// 将获取到的值类型断言为 uint64。
	return userID.(uint64), true
}

// GetUsername 从Gin上下文中获取用户名。
// 返回用户名和布尔值，表示是否成功获取。
func GetUsername(c *gin.Context) (string, bool) {
	// 从上下文中获取 "username" 键对应的值。
	username, exists := c.Get("username")
	if !exists {
		return "", false
	}
	// 将获取到的值类型断言为 string。
	return username.(string), true
}

// MustGetUserID 从Gin上下文中获取用户ID。
// 如果用户ID不存在于上下文中，则会触发 panic。
// 适用于确定用户ID必须存在于上下文的场景。
func MustGetUserID(c *gin.Context) uint64 {
	userID, exists := GetUserID(c)
	if !exists {
		panic("user_id not found in context") // 如果不存在，则抛出panic。
	}
	return userID
}
