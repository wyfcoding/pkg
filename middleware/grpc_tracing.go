package middleware

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// GrpcTracingServerOption returns a gRPC ServerOption that enables OpenTelemetry tracing
// using the stats handler mechanism.
// Note: otelgrpc v0.64.0+ prefers StatsHandler over Interceptors for better accuracy and metric support.
func GrpcTracingServerOption(opts ...otelgrpc.Option) grpc.ServerOption {
	return grpc.StatsHandler(otelgrpc.NewServerHandler(opts...))
}
