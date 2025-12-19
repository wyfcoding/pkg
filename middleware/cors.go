package middleware

import (
	"github.com/gin-gonic/gin"
)

// CORS 是一个Gin中间件，用于处理跨域资源共享 (CORS) 请求。
// 它通过设置响应头来允许来自不同源的Web应用程序访问当前服务器的资源。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 设置 Access-Control-Allow-Origin 头。
		// "*" 表示允许来自任何源的请求。在生产环境中，通常应指定具体的允许域名。
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		// 设置 Access-Control-Allow-Credentials 头为 true，表示允许发送Cookie等凭证。
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		// 设置 Access-Control-Allow-Headers 头，列出允许在实际请求中使用的自定义请求头。
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		// 设置 Access-Control-Allow-Methods 头，列出允许的HTTP方法。
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		// 对于预检请求 (OPTIONS 方法)，直接返回 204 No Content，并中止请求链。
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		// 对于非预检请求，继续执行请求链中的下一个中间件或处理器。
		c.Next()
	}
}
