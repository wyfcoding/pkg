// Package server 提供了启动和管理gRPC和HTTP服务器的封装。
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// GRPCServer 封装了标准的 grpc.Server，提供了简化的生命周期管理逻辑。
type GRPCServer struct {
	server *grpc.Server // 底层 gRPC 服务器实例.
	logger *slog.Logger // 日志记录.
	addr   string       // 监听地址 (Host:Port).
	opts   Options
}

// NewGRPCServer 构造一个新的 gRPC 协议服务器实例。
func NewGRPCServer(addr string, logger *slog.Logger, register func(*grpc.Server), interceptors []grpc.UnaryServerInterceptor, options ...Options) *GRPCServer {
	opts := Options{ShutdownTimeout: DefaultShutdownTimeout}
	if len(options) > 0 {
		opts = options[0]
	}

	var grpcOpts []grpc.ServerOption
	grpcOpts = append(grpcOpts, grpc.StatsHandler(otelgrpc.NewServerHandler()))

	if len(interceptors) > 0 {
		grpcOpts = append(grpcOpts, grpc.ChainUnaryInterceptor(interceptors...))
	}

	s := grpc.NewServer(grpcOpts...)
	register(s)
	reflection.Register(s)

	return &GRPCServer{
		server: s,
		addr:   addr,
		logger: logger,
		opts:   opts,
	}
}

// Start 启动 TCP 监听并运行 gRPC 服务。
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
		return s.Stop(context.Background())
	case err := <-errChan:
		return err
	}
}

// Stop 执行 gRPC 服务器的优雅关停。
func (s *GRPCServer) Stop(ctx context.Context) error {
	s.logger.Info("stopping grpc server gracefully")

	stopped := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(stopped)
	}()

	timer := time.NewTimer(s.opts.ShutdownTimeout)
	defer timer.Stop()

	select {
	case <-stopped:
		return nil
	case <-timer.C:
		s.logger.Warn("grpc server graceful stop timeout, forcing stop")
		s.server.Stop()
		return nil
	case <-ctx.Done():
		s.server.Stop()
		return ctx.Err()
	}
}
