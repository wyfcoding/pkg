// Package server 提供了启动和管理gRPC和HTTP服务器的封装。
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GinServer 封装了标准的 http.Server，专用于承载 Gin 引擎并提供优雅启停能力。
type GinServer struct { //nolint:govet // HTTP 服务器核心结构，已对齐。
	server *http.Server // 底层标准 HTTP 服务器。
	addr   string       // 监听地址 (Host:Port)。
	logger *slog.Logger // 日志记录器。
}

// NewGinServer 构造一个新的 Gin 协议服务器实例。
func NewGinServer(engine *gin.Engine, addr string, logger *slog.Logger) *GinServer {
	return &GinServer{
		server: &http.Server{
			Addr:              addr,
			Handler:           engine,
			ReadHeaderTimeout: 5 * time.Second,
		},
		addr:   addr,
		logger: logger,
	}
}

// Start 启动 HTTP 服务器并阻塞直到收到退出信号或发生错误。
// 流程：后台启动 ListenAndServe -> 监听 Context 信号 -> 触发优雅关闭。
func (s *GinServer) Start(ctx context.Context) error {
	s.logger.Info("starting gin server", "addr", s.addr)

	errChan := make(chan error, 1)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("gin server stopping due to context cancellation")

		const shutdownTimeout = 5 * time.Second
		shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()

		return s.server.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

// Stop 执行 Gin 服务器的优雅关停。
// 它会停止接收新请求并等待正在处理的请求完成（最长等待 5 秒）。
func (s *GinServer) Stop(ctx context.Context) error {
	s.logger.Info("stopping gin server gracefully")

	const shutdownTimeout = 5 * time.Second
	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	return s.server.Shutdown(shutdownCtx)
}
