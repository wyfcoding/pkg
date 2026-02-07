// Package middleware 提供了通用的 Gin 与 gRPC 中间件实现。
// 生成摘要:
// 1) 新增 gRPC 错误翻译拦截器，将 xerrors 映射为标准 gRPC 状态码。
// 假设:
// 1) 业务侧优先使用 xerrors 作为统一错误类型。
package middleware

import (
	"context"

	"github.com/wyfcoding/pkg/xerrors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// GRPCErrorTranslator 返回一个 gRPC 一元拦截器，用于将业务错误转换为标准 gRPC 状态码。
func GRPCErrorTranslator() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		if _, ok := status.FromError(err); ok {
			return resp, err
		}

		if xe, ok := xerrors.FromError(err); ok {
			return resp, status.Error(xe.GRPCCode(), xe.Message)
		}

		return resp, err
	}
}
