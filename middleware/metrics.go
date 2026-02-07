// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
// 生成摘要:
// 1) gRPC 指标采集支持识别 xerrors 并映射正确状态码。
// 2) 支持 HTTP/gRPC 慢请求计数指标。
// 3) 支持 HTTP 指标采集跳过指定路径。
// 假设:
// 1) 业务侧优先使用 xerrors 作为统一错误类型。
package middleware

import (
	"context"
	"strconv"
	"time"

	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/xerrors"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MetricsOptions 定义指标中间件的可选参数。
type MetricsOptions struct {
	SlowThreshold time.Duration
	SkipPaths     []string
}

// HTTPMetricsMiddleware 返回一个用于采集 HTTP 请求指标的 Gin 中间件。
func HTTPMetricsMiddleware(m *metrics.Metrics) gin.HandlerFunc {
	return HTTPMetricsMiddlewareWithOptions(m, MetricsOptions{})
}

// HTTPMetricsMiddlewareWithOptions 返回一个可配置的 HTTP 指标采集中间件。
func HTTPMetricsMiddlewareWithOptions(m *metrics.Metrics, opts MetricsOptions) gin.HandlerFunc {
	skip := make(map[string]struct{})
	for _, path := range opts.SkipPaths {
		skip[path] = struct{}{}
	}

	return func(c *gin.Context) {
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		if path == "" {
			path = "unknown"
		}
		if _, ok := skip[path]; ok {
			c.Next()
			return
		}

		if m != nil {
			m.HTTPInFlight.WithLabelValues(c.Request.Method, path).Inc()
			defer m.HTTPInFlight.WithLabelValues(c.Request.Method, path).Dec()
		}

		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		latency := time.Since(start)

		statusStr := strconv.Itoa(c.Writer.Status())

		if m != nil {
			m.HTTPRequestsTotal.WithLabelValues(c.Request.Method, path, statusStr).Inc()
			m.HTTPRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
			if opts.SlowThreshold > 0 && latency > opts.SlowThreshold {
				m.HTTPSlowRequestsTotal.WithLabelValues(c.Request.Method, path).Inc()
			}
		}
	}
}

// GRPCMetricsInterceptor 返回一个用于采集 gRPC 请求指标的一元拦截器。
func GRPCMetricsInterceptor(m *metrics.Metrics) grpc.UnaryServerInterceptor {
	return GRPCMetricsInterceptorWithOptions(m, MetricsOptions{})
}

// GRPCMetricsInterceptorWithOptions 返回一个可配置的 gRPC 指标采集拦截器。
func GRPCMetricsInterceptorWithOptions(m *metrics.Metrics, opts MetricsOptions) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if m != nil {
			m.GRPCInFlight.WithLabelValues("server", info.FullMethod).Inc()
			defer m.GRPCInFlight.WithLabelValues("server", info.FullMethod).Dec()
		}

		start := time.Now()

		resp, err := handler(ctx, req)

		elapsed := time.Since(start)
		duration := elapsed.Seconds()
		code := codes.OK
		if err != nil {
			if xe, ok := xerrors.FromError(err); ok {
				code = xe.GRPCCode()
			} else {
				st, _ := status.FromError(err)
				code = st.Code()
			}
		}

		if m != nil {
			m.GRPCRequestsTotal.WithLabelValues("server", info.FullMethod, code.String()).Inc()
			m.GRPCRequestDuration.WithLabelValues("server", info.FullMethod).Observe(duration)
			if opts.SlowThreshold > 0 && elapsed > opts.SlowThreshold {
				m.GRPCSlowRequestsTotal.WithLabelValues("server", info.FullMethod).Inc()
			}
		}

		return resp, err
	}
}
