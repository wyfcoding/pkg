// Package server 提供了启动和管理gRPC和HTTP服务器的封装。
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// GRPCServer 封装了标准的 grpc.Server，提供了简化的生命周期管理逻辑。
type GRPCServer struct {
	server *grpc.Server // 底层 gRPC 服务器实例.
	logger *slog.Logger // 日志记录.
	addr   string       // 监听地址 (Host:Port).
}

// NewGRPCServer 构造一个新的 gRPC 协议服务器实例。
// 核心特性.
// 1. 自动启用 OpenTelemetry StatsHandler 进行全链路监控。
// 2. 支持自定义拦截器链。
// 3. 自动开启 gRPC Reflection（反射服务），便于调试与工具集成。
func NewGRPCServer(addr string, logger *slog.Logger, register func(*grpc.Server), interceptors ...grpc.UnaryServerInterceptor) *GRPCServer {
	var opts []grpc.ServerOption

	opts = append(opts, grpc.StatsHandler(otelgrpc.NewServerHandler()))

	if len(interceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(interceptors...))
	}

	s := grpc.NewServer(opts...)
	register(s)
	reflection.Register(s)

	return &GRPCServer{
		server: s,
		addr:   addr,
		logger: logger,
	}
}

// Start 启动 TCP 监听并运行 gRPC 服务。
// 流程：建立 TCP 监听 -> 启动 Serve 协程 -> 监听信号。
func (s *GRPCServer) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}
	s.logger.Info("starting grpc server", "addr", s.addr)

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.server.Serve(lis)
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("grpc server stopping due to context cancellation")
		s.server.Stop()
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

// Stop 执行 gRPC 服务器的优雅关停。
// 它会等待所有活跃的 RPC 调用完成后再退出。
func (s *GRPCServer) Stop(_ context.Context) error {
	s.logger.Info("stopping grpc server gracefully")
	s.server.GracefulStop()
	return nil
}
