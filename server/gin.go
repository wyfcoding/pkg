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

const (
	DefaultShutdownTimeout = 10 * time.Second
)

// Options 定义服务器选项。
type Options struct {
	ShutdownTimeout time.Duration
}

// GinServer 封装了标准的 http.Server，专用于承载 Gin 引擎并提供优雅启停能力。
type GinServer struct {
	server *http.Server // 底层标准 HTTP 服务器。
	logger *slog.Logger // 日志记录器。
	addr   string       // 监听地址 (Host:Port)。
	opts   Options
}

// NewGinServer 构造一个新的 Gin 协议服务器实例。
func NewGinServer(engine *gin.Engine, addr string, logger *slog.Logger, options ...Options) *GinServer {
	opts := Options{ShutdownTimeout: DefaultShutdownTimeout}
	if len(options) > 0 {
		opts = options[0]
	}

	return &GinServer{
		server: &http.Server{
			Addr:              addr,
			Handler:           engine,
			ReadHeaderTimeout: 5 * time.Second,
		},
		addr:   addr,
		logger: logger,
		opts:   opts,
	}
}

// Start 启动 HTTP 服务器并阻塞直到收到退出信号或发生错误。
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
		return s.Stop(context.Background()) // 使用背景上下文触发 Stop
	case err := <-errChan:
		return err
	}
}

// Stop 执行 Gin 服务器的优雅关停。
func (s *GinServer) Stop(ctx context.Context) error {
	s.logger.Info("stopping gin server gracefully")

	shutdownCtx, cancel := context.WithTimeout(ctx, s.opts.ShutdownTimeout)
	defer cancel()

	return s.server.Shutdown(shutdownCtx)
}
