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

// HTTPMetricsMiddleware (Gin)
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

		m.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, statusStr).Inc()
		m.HTTPRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}

// GrpcMetricsInterceptor (gRPC Server)
func GrpcMetricsInterceptor(m *metrics.Metrics) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start).Seconds()

		st, _ := status.FromError(err)
		m.GRPCRequestsTotal.WithLabelValues("server", info.FullMethod, st.Code().String()).Inc()
		m.GRPCRequestDuration.WithLabelValues("server", info.FullMethod).Observe(duration)

		return resp, err
	}
}
