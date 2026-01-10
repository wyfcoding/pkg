package async

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
)

var (
	// ErrPanicRecovered 表示异步任务中恢复的 panic。
	ErrPanicRecovered = errors.New("async task panic recovered")
)

// Runner 定义了安全的并发执行器接口。
type Runner interface {
	// Go 安全地启动一个 goroutine，自动处理 panic。
	Go(fn func())
	// GoWithContext 安全地启动一个 goroutine，并注入 context。
	GoWithContext(ctx context.Context, fn func(ctx context.Context))
}

// defaultRunner 是默认的安全执行器。
type defaultRunner struct {
	logger *slog.Logger
}

var DefaultRunner = &defaultRunner{
	logger: slog.Default(),
}

// Go 安全地启动一个 goroutine，自动处理 panic。
func (r *defaultRunner) Go(fn func()) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				r.logPanic(rec)
			}
		}()
		fn()
	}()
}

// GoWithContext 安全地启动一个 goroutine，并注入 context。
func (r *defaultRunner) GoWithContext(ctx context.Context, fn func(ctx context.Context)) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				r.logPanic(rec)
			}
		}()
		fn(ctx)
	}()
}

func (r *defaultRunner) logPanic(rec any) {
	err := fmt.Errorf("%w: %v", ErrPanicRecovered, rec)
	stack := string(debug.Stack())
	r.logger.Error("Async task panic recovered", "error", err, "stack", stack)
}

// SafeGo 是 DefaultRunner.Go 的快捷方式。
func SafeGo(fn func()) {
	DefaultRunner.Go(fn)
}

// RunGroup 类似于 errgroup，但增加了 panic 恢复。
type RunGroup struct {
	err     error
	wg      sync.WaitGroup
	errOnce sync.Once
}

// Go 在组中启动一个任务。
func (g *RunGroup) Go(fn func() error) {
	g.wg.Add(1)
	SafeGo(func() {
		defer g.wg.Done()
		if err := fn(); err != nil {
			g.errOnce.Do(func() {
				g.err = err
			})
		}
	})
}

// Wait 等待所有任务完成，并返回第一个错误（如果有）。
func (g *RunGroup) Wait() error {
	g.wg.Wait()
	return g.err
}
