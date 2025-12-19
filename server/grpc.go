// Package server 提供了启动和管理gRPC和HTTP服务器的封装。
package server

import (
	"context"
	"log/slog"
	"net"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// GRPCServer 封装了标准的 `grpc.Server`，提供了简化的启动和停止逻辑。
type GRPCServer struct {
	server *grpc.Server
	addr   string
	logger *slog.Logger
}

// NewGRPCServer 创建一个新的GRPC服务器实例。
// 它会自动注册服务、gRPC反射，并应用传入的拦截器。
// 它还会默认启用 OpenTelemetry StatsHandler 进行全链路追踪。
func NewGRPCServer(addr string, logger *slog.Logger, register func(*grpc.Server), interceptors ...grpc.UnaryServerInterceptor) *GRPCServer {
	var opts []grpc.ServerOption
	
	// 默认启用 OpenTelemetry StatsHandler
	opts = append(opts, grpc.StatsHandler(otelgrpc.NewServerHandler()))

	// 如果有提供拦截器，则构建一元拦截器链
	if len(interceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(interceptors...))
	}

	// 创建gRPC服务器
	s := grpc.NewServer(opts...)

	// 调用传入的函数来注册gRPC服务
	register(s)

	// 在gRPC服务器上注册反射服务。这允许gRPC客户端（如grpcurl）查询服务所暴露的RPC。
	reflection.Register(s)

	return &GRPCServer{
		server: s,
		addr:   addr,
		logger: logger,
	}
}

// Start 启动gRPC服务器并监听指定地址。
// 这是一个阻塞操作，直到上下文被取消或服务出错。
func (s *GRPCServer) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.logger.Info("Starting gRPC server", "addr", s.addr)

	errChan := make(chan error, 1)
	// 在一个新的goroutine中启动服务器，以免阻塞主线程
	go func() {
		errChan <- s.server.Serve(lis)
	}()

	// 监听上下文取消信号或服务器错误
	select {
	case <-ctx.Done():
		// 如果上下文被取消，则停止服务器
		s.logger.Info("gRPC server stopping due to context cancellation.")
		s.server.Stop() // 立即停止
		return ctx.Err()
	case err := <-errChan:
		// 如果服务器启动或运行中出错，则返回错误
		return err
	}
}

// Stop 优雅地停止gRPC服务器。
// 它会等待现有连接完成，但不会接受新连接。
func (s *GRPCServer) Stop(ctx context.Context) error {
	s.logger.Info("Stopping gRPC server gracefully")
	s.server.GracefulStop()
	return nil
}