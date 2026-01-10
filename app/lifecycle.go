// Package app 提供了应用程序生命周期管理的基础设施。
package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Hook 定义了生命周期钩子，包含启动和停止逻辑。
type Hook struct {
	OnStart func(context.Context) error
	OnStop  func(context.Context) error
	Name    string
}

// Lifecycle 管理应用程序中多个组件的生命周期。
type Lifecycle struct {
	hooks  []Hook
	mu     sync.Mutex
	logger *slog.Logger
}

// NewLifecycle 创建一个新的生命周期管理器实例。
func NewLifecycle(logger *slog.Logger) *Lifecycle {
	return &Lifecycle{
		hooks:  make([]Hook, 0),
		logger: logger.With("module", "lifecycle"),
	}
}

// Append 添加一个生命周期钩子。
func (l *Lifecycle) Append(hook Hook) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.hooks = append(l.hooks, hook)
}

// Start 按顺序启动所有组件。
func (l *Lifecycle) Start(ctx context.Context) error {
	for _, hook := range l.hooks {
		if hook.OnStart != nil {
			l.logger.Debug("Lifecycle: starting component", "name", hook.Name)

			if err := hook.OnStart(ctx); err != nil {
				l.logger.Error("Lifecycle: failed to start component", "name", hook.Name, "error", err)

				return fmt.Errorf("failed to start %s: %w", hook.Name, err)
			}
		}
	}

	return nil
}

// Stop 以相反的顺序停止所有组件。
func (l *Lifecycle) Stop(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 反向停止。
	for i := len(l.hooks) - 1; i >= 0; i-- {
		hook := l.hooks[i]
		if hook.OnStop != nil {
			l.logger.Debug("Lifecycle: stopping component", "name", hook.Name)

			if err := hook.OnStop(ctx); err != nil {
				l.logger.Error("Lifecycle: failed to stop component", "name", hook.Name, "error", err)
				// 停止失败通常继续停止其他组件。
			}
		}
	}

	return nil
}
