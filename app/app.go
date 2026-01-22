// Package app 提供了应用程序的构建和管理功能.
package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultShutdownTimeout = 15 * time.Second
)

// App 是应用程序的核心容器.
type App struct {
	logger    *slog.Logger
	lifecycle *Lifecycle
	name      string
}

// New 创建一个新的应用程序实例.
func New(name string, logger *slog.Logger, opts ...Option) *App {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	l := NewLifecycle(logger)

	// 注册服务器
	for _, srv := range o.servers {
		l.Append(Hook{
			Name: "Server",
			OnStart: func(ctx context.Context) error {
				go func() {
					if err := srv.Start(ctx); err != nil {
						logger.Error("server start error", "error", err)
					}
				}()
				return nil
			},
			OnStop: func(ctx context.Context) error {
				return srv.Stop(ctx)
			},
		})
	}

	// 注册清理函数
	for _, cleanup := range o.cleanups {
		l.Append(Hook{
			Name: "Cleanup",
			OnStop: func(ctx context.Context) error {
				cleanup()
				return nil
			},
		})
	}

	return &App{
		name:      name,
		logger:    logger,
		lifecycle: l,
	}
}

// Run 启动应用程序.
func (a *App) Run() error {
	a.printBanner()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := a.lifecycle.Start(ctx); err != nil {
		return err
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	a.logger.Info("shutting down application", "name", a.name)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer stopCancel()

	return a.lifecycle.Stop(stopCtx)
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
	a.logger.Info(banner)
	a.logger.Info("Application starting...", "name", a.name, "pid", os.Getpid())
}
