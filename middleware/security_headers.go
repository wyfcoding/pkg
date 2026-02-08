package middleware

import (
	"strconv"
	"strings"

	"github.com/wyfcoding/pkg/config"

	"github.com/gin-gonic/gin"
)

const defaultSecurityHeaderSeparator = ":"

// SecurityHeadersWithConfig 返回 HTTP 安全响应头中间件。
func SecurityHeadersWithConfig(cfg config.SecurityConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		cfg = withSecurityDefaults(cfg)

		setHeaderIfValue(c, "X-Frame-Options", cfg.FrameOptions)
		setHeaderIfValue(c, "X-Content-Type-Options", cfg.ContentTypeOptions)
		setHeaderIfValue(c, "X-XSS-Protection", cfg.XSSProtection)
		setHeaderIfValue(c, "Referrer-Policy", cfg.ReferrerPolicy)
		setHeaderIfValue(c, "Content-Security-Policy", cfg.ContentSecurityPolicy)
		setHeaderIfValue(c, "Permissions-Policy", cfg.PermissionsPolicy)

		if cfg.HSTSMaxAge > 0 {
			value := "max-age=" + strconv.Itoa(cfg.HSTSMaxAge)
			if cfg.HSTSIncludeSubdomains {
				value += "; includeSubDomains"
			}
			if cfg.HSTSPreload {
				value += "; preload"
			}
			setHeaderIfValue(c, "Strict-Transport-Security", value)
		}

		applyAdditionalHeaders(c, cfg)

		c.Next()
	}
}

func withSecurityDefaults(cfg config.SecurityConfig) config.SecurityConfig {
	if cfg.FrameOptions == "" {
		cfg.FrameOptions = "DENY"
	}
	if cfg.ContentTypeOptions == "" {
		cfg.ContentTypeOptions = "nosniff"
	}
	if cfg.XSSProtection == "" {
		cfg.XSSProtection = "0"
	}
	if cfg.ReferrerPolicy == "" {
		cfg.ReferrerPolicy = "no-referrer"
	}
	return cfg
}

func applyAdditionalHeaders(c *gin.Context, cfg config.SecurityConfig) {
	if len(cfg.AdditionalHeaders) == 0 {
		return
	}

	sep := cfg.AdditionalHeaderSeparator
	if sep == "" {
		sep = defaultSecurityHeaderSeparator
	}

	for _, item := range cfg.AdditionalHeaders {
		parts := strings.SplitN(item, sep, 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		setHeaderIfValue(c, key, val)
	}
}

func setHeaderIfValue(c *gin.Context, key, value string) {
	if value == "" {
		return
	}
	c.Header(key, value)
}
