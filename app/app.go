// Package app 提供了应用程序的构建和管理功能，包括服务的启动、停止和资源清理。
package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wyfcoding/pkg/server" // 导入服务器接口定义。
)

// App 是应用程序的核心容器，负责管理应用程序的生命周期。
// 包括注册和启动服务器、处理信号量、以及优雅地关闭所有资源。
type App struct {
	name   string          // 应用程序的名称。
	logger *slog.Logger    // 应用程序的日志记录器。
	opts   options         // 应用程序的选项，包含服务器和清理函数列表。
	ctx    context.Context // 应用程序的根上下文，用于控制生命周期。
	cancel func()          // 取消函数，调用它会取消 a.ctx。
}

// New 创建一个新的应用程序实例。
// name: 应用程序的唯一标识名称。
// logger: 应用程序使用的日志记录器。
// opts: 可选的配置选项，用于注册服务器和清理函数。
func New(name string, logger *slog.Logger, opts ...Option) *App {
	o := options{}
	// 遍历并应用所有传入的Option函数，配置App实例的选项。
	for _, opt := range opts {
		opt(&o)
	}

	// 创建一个可取消的上下文，作为应用程序的根上下文。
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		name:   name,
		logger: logger,
		opts:   o,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Run 启动应用程序。
// 它会启动所有注册的服务器，监听操作系统信号进行优雅关闭，并在收到信号后执行清理操作。
// 这是一个阻塞函数，直到应用程序被关闭。
func (a *App) Run() error {
	a.printBanner()

	// 启动所有注册的服务器。每个服务器都在自己的goroutine中启动。
	for _, srv := range a.opts.servers {
		go func(s server.Server) {
			// server.Start是一个阻塞调用，直到上下文被取消或发生错误。
			if err := s.Start(a.ctx); err != nil {
				a.logger.Error("server failed to start", "error", err)
				// 如果任何一个关键服务器启动失败，则取消应用程序上下文，触发关闭流程。
				a.cancel()
			}
		}(srv)
	}

	// 监听操作系统信号，用于优雅关闭。
	quit := make(chan os.Signal, 1) // 创建一个信号通道。
	// 注册要监听的信号：中断信号（SIGINT, Ctrl+C）和终止信号（SIGTERM）。
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit // 阻塞等待直到接收到信号。
	a.logger.Info("shutting down application", "name", a.name)

	// 收到退出信号后，调用cancel函数，通知所有监听 `a.ctx` 的goroutine停止。
	if a.cancel != nil {
		a.cancel()
	}

	// 创建一个带有超时机制的上下文，用于控制服务器的关闭时间，防止无限等待。
	// 默认超时5秒，可通过Option配置（后续优化.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel() // 确保在函数退出时取消此上下文。

	// 优雅地停止所有服务器。
	for _, srv := range a.opts.servers {
		if err := srv.Stop(shutdownCtx); err != nil {
			a.logger.Error("server failed to stop", "error", err)
			return err // 如果服务器未能优雅关闭，则返回错误。
		}
	}

	// 执行所有注册的清理函数。
	for _, cleanup := range a.opts.cleanups {
		cleanup()
	}

	a.logger.Info("application shut down gracefully")
	return nil
}

func (a *App) printBanner() {
	const banner = `
 __          __   __     __   ________ 
 \ \        / /   \ \   / /  |  ______|
  \ \  /\  / /     \ \_/ /   | |__     
   \ \/  \/ /       \   /    |  __|    
    \  /\  /         | |     | |       
     \/  \/          |_|     |_|       
`
	// 简单打印 Banner，不依赖外部库。后续可以加入更炫酷的ASCII Art。
	// 这里使用 println 确保在日志初始化前也能看到，或者使用 logger.Inf.
	// 鉴于 logger 已经初始化，使用 logger.Inf.
	a.logger.Info(banner)
	a.logger.Info("Application starting...", "name", a.name, "pid", os.Getpid())
}
