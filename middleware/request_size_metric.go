package middleware

import (
	"github.com/wyfcoding/pkg/metrics"

	"github.com/gin-gonic/gin"
)

// HTTPRequestSizeMiddleware 记录请求体大小指标。
func HTTPRequestSizeMiddleware(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		if m == nil {
			c.Next()
			return
		}

		c.Next()

		if m.HTTPRequestSizeBytes == nil {
			m.RegisterRequestSizeMetrics()
		}

		size := requestSize(c)
		if m.HTTPRequestSizeBytes != nil {
			path := c.FullPath()
			if path == "" {
				path = c.Request.URL.Path
			}
			m.HTTPRequestSizeBytes.WithLabelValues(c.Request.Method, path).Observe(float64(size))
		}
	}
}

func requestSize(c *gin.Context) int64 {
	if c == nil || c.Request == nil {
		return 0
	}
	if c.Request.ContentLength > 0 {
		return c.Request.ContentLength
	}
	return 0
}
