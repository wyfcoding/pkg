// Package middleware 提供了通用的 Gin 与 gRPC 中间件实现。
// 生成摘要:
// 1) 新增 gRPC 上下文增强拦截器，自动注入 IP/UA/租户信息。
// 假设:
// 1) 租户 ID 使用 metadata "x-tenant-id" 传递。
package middleware

import (
	"context"

	"github.com/wyfcoding/pkg/contextx"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

const grpcTenantIDKey = "x-tenant-id"
const grpcUserAgentKey = "user-agent"

// GRPCContextEnricher 返回一个 gRPC 一元拦截器，用于注入常用上下文字段。
func GRPCContextEnricher() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
			ctx = contextx.WithIP(ctx, p.Addr.String())
		}

		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(grpcTenantIDKey); len(vals) > 0 {
				ctx = contextx.WithTenantID(ctx, vals[0])
			}
			if vals := md.Get(grpcUserAgentKey); len(vals) > 0 {
				ctx = contextx.WithUserAgent(ctx, vals[0])
			}
		}

		return handler(ctx, req)
	}
}
