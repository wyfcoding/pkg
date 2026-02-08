package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultCORSMaxAge = 12 * time.Hour

// CORSOptions 定义跨域访问的配置项。
type CORSOptions struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           time.Duration
}

// CORS 是一个 Gin 中间件，用于处理跨域资源共享 (CORS) 请求。
func CORS() gin.HandlerFunc {
	return CORSWithOptions(CORSOptions{})
}

// CORSWithOptions 构造带配置项的 CORS 中间件。
func CORSWithOptions(opts CORSOptions) gin.HandlerFunc {
	opts = normalizeCORSOptions(opts)

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		allowedOrigin, ok := allowOrigin(origin, opts.AllowOrigins, opts.AllowCredentials)
		if !ok {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		headers := c.Writer.Header()
		headers.Set("Access-Control-Allow-Origin", allowedOrigin)
		headers.Add("Vary", "Origin")
		headers.Set("Access-Control-Allow-Methods", strings.Join(opts.AllowMethods, ", "))
		headers.Set("Access-Control-Allow-Headers", strings.Join(opts.AllowHeaders, ", "))
		if len(opts.ExposeHeaders) > 0 {
			headers.Set("Access-Control-Expose-Headers", strings.Join(opts.ExposeHeaders, ", "))
		}
		if opts.AllowCredentials {
			headers.Set("Access-Control-Allow-Credentials", "true")
		}
		if opts.MaxAge > 0 {
			headers.Set("Access-Control-Max-Age", strconv.FormatInt(int64(opts.MaxAge.Seconds()), 10))
		}

		if c.Request.Method == http.MethodOptions {
			if c.GetHeader("Access-Control-Request-Method") != "" {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}

		c.Next()
	}
}

func normalizeCORSOptions(opts CORSOptions) CORSOptions {
	if len(opts.AllowOrigins) == 0 {
		opts.AllowOrigins = []string{"*"}
	}
	if len(opts.AllowMethods) == 0 {
		opts.AllowMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions}
	}
	if len(opts.AllowHeaders) == 0 {
		opts.AllowHeaders = []string{"Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization", "accept", "origin", "Cache-Control", "X-Requested-With"}
	}
	if opts.MaxAge == 0 {
		opts.MaxAge = defaultCORSMaxAge
	}
	return opts
}

func allowOrigin(origin string, allowed []string, allowCredentials bool) (string, bool) {
	for _, item := range allowed {
		if item == "*" {
			if allowCredentials {
				return origin, true
			}
			return "*", true
		}
		if strings.EqualFold(item, origin) {
			return origin, true
		}
	}
	return "", false
}
