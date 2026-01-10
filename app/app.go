// Package app 提供了应用程序的构建和管理功能.
package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wyfcoding/pkg/server"
)

const (
	defaultShutdownTimeout = 10 * time.Second
)

// App 是应用程序的核心容器.
type App struct {
	logger *slog.Logger
	name   string
	opts   options
}

// New 创建一个新的应用程序实例.
func New(name string, logger *slog.Logger, opts ...Option) *App {
	o := options{
		servers:        nil,
		cleanups:       nil,
		healthCheckers: nil,
	}

	for _, opt := range opts {
		opt(&o)
	}

	return &App{
		name:   name,
		logger: logger,
		opts:   o,
	}
}

// Run 启动应用程序.
func (a *App) Run() error {
	a.printBanner()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, srv := range a.opts.servers {
		go func(s server.Server) {
			if err := s.Start(ctx); err != nil {
				a.logger.Error("server failed to start", "error", err)
				cancel()
			}
		}(srv)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	a.logger.Info("shutting down application", "name", a.name)

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
	defer shutdownCancel()

	for _, srv := range a.opts.servers {
		if err := srv.Stop(shutdownCtx); err != nil {
			a.logger.Error("server failed to stop", "error", err)

			return err
		}
	}

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
	a.logger.Info(banner)
	a.logger.Info("Application starting...", "name", a.name, "pid", os.Getpid())
}
