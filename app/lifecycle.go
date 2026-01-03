package app

import (
	"context"
	"log/slog"
	"sync"
)

// Hook 定义了生命周期钩子，包含启动和停止逻辑
type Hook struct {
	Name    string
	OnStart func(ctx context.Context) error
	OnStop  func(ctx context.Context) error
}

// Lifecycle 管理应用程序中多个组件的生命周期
type Lifecycle struct {
	logger *slog.Logger
	hooks  []Hook
	mu     sync.Mutex
}

// NewLifecycle 创建一个新的生命周期管理器
func NewLifecycle(logger *slog.Logger) *Lifecycle {
	return &Lifecycle{
		logger: logger,
	}
}

// Append 添加一个生命周期钩子
func (l *Lifecycle) Append(hook Hook) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hooks = append(l.hooks, hook)
}

// Start 按顺序启动所有组件
func (l *Lifecycle) Start(ctx context.Context) error {
	for _, hook := range l.hooks {
		if hook.OnStart != nil {
			l.logger.Info("Lifecycle: starting component", "name", hook.Name)
			if err := hook.OnStart(ctx); err != nil {
				l.logger.Error("Lifecycle: failed to start component", "name", hook.Name, "error", err)
				return err
			}
		}
	}
	return nil
}

// Stop 以相反的顺序停止所有组件
func (l *Lifecycle) Stop(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var firstErr error
	for i := len(l.hooks) - 1; i >= 0; i-- {
		hook := l.hooks[i]
		if hook.OnStop != nil {
			l.logger.Info("Lifecycle: stopping component", "name", hook.Name)
			if err := hook.OnStop(ctx); err != nil {
				l.logger.Error("Lifecycle: failed to stop component", "name", hook.Name, "error", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}
