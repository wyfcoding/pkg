// Package middleware 提供了通用的 Gin 与 gRPC 中间件实现。
// 生成摘要:
// 1) gRPC 访问日志拦截器支持识别 xerrors 并映射正确状态码。
// 2) 支持慢请求阈值配置。
// 3) 支持从上下文中自动透传 request_id/trace_id 等字段。
// 假设:
// 1) gRPC 状态码为 OK 时记录 Info，其余按严重度提升日志级别。
package middleware

import (
	"context"
	"time"

	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/xerrors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// GRPCRequestLoggerOptions 定义 gRPC 访问日志拦截器的参数。
type GRPCRequestLoggerOptions struct {
	SlowThreshold time.Duration
}

const defaultGRPCSlowThreshold = 500 * time.Millisecond

// GRPCRequestLogger 返回一个 gRPC 一元拦截器，用于记录请求的耗时与状态。
func GRPCRequestLogger() grpc.UnaryServerInterceptor {
	return GRPCRequestLoggerWithOptions(GRPCRequestLoggerOptions{SlowThreshold: defaultGRPCSlowThreshold})
}

// GRPCRequestLoggerWithOptions 返回一个可配置的 gRPC 请求日志拦截器。
func GRPCRequestLoggerWithOptions(opts GRPCRequestLoggerOptions) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		slowThreshold := opts.SlowThreshold
		if slowThreshold <= 0 {
			slowThreshold = defaultGRPCSlowThreshold
		}

		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start)
		code := codes.OK
		if err != nil {
			if xe, ok := xerrors.FromError(err); ok {
				code = xe.GRPCCode()
			} else {
				st, _ := status.FromError(err)
				code = st.Code()
			}
		}

		fields := []any{
			"method", info.FullMethod,
			"status", code.String(),
			"duration", duration,
		}

		if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
			fields = append(fields, "peer", p.Addr.String())
		}

		if err != nil {
			fields = append(fields, "error", err)
		}

		switch code {
		case codes.OK:
			if duration > slowThreshold {
				logging.Warn(ctx, "grpc request slow", fields...)
			} else {
				logging.Info(ctx, "grpc request processed", fields...)
			}
		case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists, codes.FailedPrecondition, codes.PermissionDenied, codes.Unauthenticated:
			logging.Warn(ctx, "grpc request client error", fields...)
		default:
			logging.Error(ctx, "grpc request server error", fields...)
		}

		return resp, err
	}
}
