package middleware

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// GrpcTracingServerOption returns a gRPC ServerOption that enables OpenTelemetry tracing
// 使用 stats handler 机制。
// 注意：otelgrpc v0.64.0+ 更倾向于使用 StatsHandler 而不是 Interceptors，以获得更好的精度和指标支持。
func GrpcTracingServerOption(opts ...otelgrpc.Option) grpc.ServerOption {
	return grpc.StatsHandler(otelgrpc.NewServerHandler(opts...))
}
