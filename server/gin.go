// Package server 提供了启动和管理gRPC和HTTP服务器的封装。
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GinServer 封装了标准的 `http.Server`，专门用于运行 Gin 引擎，并提供了优雅的启动和关闭功能。
type GinServer struct {
	server *http.Server
	addr   string
	logger *slog.Logger
}

// NewGinServer 创建一个新的Gin服务器实例。
func NewGinServer(engine *gin.Engine, addr string, logger *slog.Logger) *GinServer {
	return &GinServer{
		server: &http.Server{
			Addr:    addr,
			Handler: engine, // 将Gin引擎作为HTTP处理器
		},
		addr:   addr,
		logger: logger,
	}
}

// Start 启动Gin HTTP服务器。
// 这是一个阻塞操作，它会监听上下文的取消事件以触发优雅关闭。
func (s *GinServer) Start(ctx context.Context) error {
	s.logger.Info("Starting Gin server", "addr", s.addr)

	errChan := make(chan error, 1)
	// 在goroutine中启动HTTP服务器
	go func() {
		// ListenAndServe会阻塞，直到服务器关闭。
		// 如果不是因为正常的ServerClosed错误而退出，我们就将其视为一个真正的错误。
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// 监听上下文取消或服务器启动错误
	select {
	case <-ctx.Done():
		// 上下文被取消，开始优雅关闭
		s.logger.Info("Gin server stopping due to context cancellation.")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // 创建一个带超时的关闭上下文
		defer cancel()
		return s.server.Shutdown(shutdownCtx) // 执行优雅关闭
	case err := <-errChan:
		// 返回服务器启动时遇到的错误
		return err
	}
}

// Stop 优雅地停止Gin服务器。
// 它会等待现有请求在给定超时时间内完成。
func (s *GinServer) Stop(ctx context.Context) error {
	s.logger.Info("Stopping Gin server gracefully")
	// 创建一个有超时的上下文，以防关闭过程耗时过长
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// 调用http.Server的Shutdown方法进行优雅关闭
	return s.server.Shutdown(ctx)
}
