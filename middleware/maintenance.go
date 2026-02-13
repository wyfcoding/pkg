package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/response"

	"github.com/gin-gonic/gin"
)

var defaultMaintenanceAllowPaths = []string{"/sys/health", "/sys/ready", "/sys/live", "/metrics"}

// MaintenanceMiddleware 启用维护模式时直接拒绝请求。
func MaintenanceMiddleware(cfg config.MaintenanceConfig) gin.HandlerFunc {
	allowed := mergeAllowedPaths(defaultMaintenanceAllowPaths, cfg.AllowPaths)

	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		if isAllowedPath(path, allowed) {
			c.Next()
			return
		}

		message := cfg.Message
		if message == "" {
			message = "Service is under maintenance"
		}

		response.ErrorWithStatus(c, http.StatusServiceUnavailable, message, "maintenance mode")
		c.Abort()
	}
}

func mergeAllowedPaths(defaults, extra []string) []string {
	if len(extra) == 0 {
		return defaults
	}

	seen := make(map[string]struct{}, len(defaults)+len(extra))
	result := make([]string, 0, len(defaults)+len(extra))
	for _, path := range defaults {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	for _, path := range extra {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}

func isAllowedPath(path string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	return slices.Contains(allowed, path)
}
