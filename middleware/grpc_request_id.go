// Package middleware 提供了通用的 Gin 与 gRPC 中间件实现。
// 生成摘要:
// 1) 新增 gRPC Request ID 生成与传递拦截器。
// 2) 将 request_id 注入 context，并附带 trace_id 响应头。
// 假设:
// 1) gRPC metadata 使用键 "x-request-id" 传递请求 ID。
package middleware

import (
	"context"

	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/idgen"
	"github.com/wyfcoding/pkg/tracing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const grpcRequestIDKey = "x-request-id"
const grpcTraceIDKey = "x-trace-id"

// GRPCRequestID 返回一个 gRPC 一元拦截器，用于生成或提取 Request ID 并注入上下文。
func GRPCRequestID() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		requestID := ""

		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(grpcRequestIDKey); len(vals) > 0 {
				requestID = vals[0]
			}
		}

		if requestID == "" {
			requestID = idgen.GenIDString()
		}

		newCtx := contextx.WithRequestID(ctx, requestID)
		_ = grpc.SetHeader(newCtx, metadata.Pairs(grpcRequestIDKey, requestID))

		if traceID := tracing.GetTraceID(newCtx); traceID != "" {
			_ = grpc.SetHeader(newCtx, metadata.Pairs(grpcTraceIDKey, traceID))
		}

		return handler(newCtx, req)
	}
}
