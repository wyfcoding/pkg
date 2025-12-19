// Package middleware 提供了Gin和gRPC的通用中间件。
package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// httpRequestsTotal 是一个Prometheus计数器，用于记录HTTP请求的总量。
	// 标签包括请求方法（method）、请求路径（path）和HTTP状态码（status）。
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",           // 指标名称
			Help: "Total number of HTTP requests", // 指标说明
		},
		[]string{"method", "path", "status"}, // 标签
	)
	// httpRequestDuration 是一个Prometheus直方图，用于记录HTTP请求的处理耗时。
	// 标签包括请求方法（method）和请求路径（path）。
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",    // 指标名称
			Help:    "HTTP request duration in seconds", // 指标说明
			Buckets: prometheus.DefBuckets,              // 使用Prometheus默认的直方图分桶
		},
		[]string{"method", "path"}, // 标签
	)
)

// init 函数在包加载时自动执行，用于注册Prometheus指标。
func init() {
	prometheus.MustRegister(httpRequestsTotal, httpRequestDuration)
}

// MetricsMiddleware 创建一个Gin中间件，用于收集HTTP请求相关的Prometheus指标。
// 它会记录每个请求的总数和处理耗时。
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()                     // 记录请求开始时间
		c.Next()                                // 处理请求链中的下一个中间件或处理器
		duration := time.Since(start).Seconds() // 计算请求处理耗时

		status := strconv.Itoa(c.Writer.Status()) // 获取HTTP响应状态码并转换为字符串
		path := c.FullPath()                      // 获取请求的完整路径（路由模板）
		if path == "" {
			path = "unknown" // 如果没有匹配到路由模板，则标记为unknown
		}

		// 增加HTTP请求总数计数
		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		// 记录HTTP请求处理耗时
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}
