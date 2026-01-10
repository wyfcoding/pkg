// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
package middleware

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// HTTPMetricsMiddleware 返回一个用于采集 HTTP 请求指标的 Gin 中间件。
func HTTPMetricsMiddleware(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		path := c.FullPath()

		if path == "" {
			path = "unknown"
		}

		statusStr := strconv.Itoa(c.Writer.Status())

		if m != nil {
			m.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, statusStr).Inc()
			m.HTTPRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
		}
	}
}

// GRPCMetricsInterceptor 返回一个用于采集 gRPC 请求指标的一元拦截器。
func GRPCMetricsInterceptor(m *metrics.Metrics) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start).Seconds()
		st, _ := status.FromError(err)

		if m != nil {
			m.GRPCRequestsTotal.WithLabelValues("server", info.FullMethod, st.Code().String()).Inc()
			m.GRPCRequestDuration.WithLabelValues("server", info.FullMethod).Observe(duration)
		}

		return resp, err
	}
}