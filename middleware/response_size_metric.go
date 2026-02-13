package middleware

import (
	"github.com/wyfcoding/pkg/metrics"

	"github.com/gin-gonic/gin"
)

// HTTPResponseSizeMiddleware 记录响应体大小指标。
func HTTPResponseSizeMiddleware(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		if m == nil {
			c.Next()
			return
		}

		c.Next()

		if m.HTTPResponseSizeBytes == nil {
			m.RegisterResponseSizeMetrics()
		}
		if m.HTTPResponseSizeBytes == nil {
			return
		}

		size := max(c.Writer.Size(), 0)
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		m.HTTPResponseSizeBytes.WithLabelValues(c.Request.Method, path).Observe(float64(size))
	}
}
