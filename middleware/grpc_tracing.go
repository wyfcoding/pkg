// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
package middleware

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// GRPCTracingServerOption 返回启用 OpenTelemetry 链路追踪的 gRPC ServerOption。
func GRPCTracingServerOption() grpc.ServerOption {
	return grpc.StatsHandler(otelgrpc.NewServerHandler())
}
