// Package saga 提供了分布式事务编排的通用接口与实现。
// 增强：提供了一个轻量级的状态机驱动编排器，支持本地与远程补偿逻辑。
package saga

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wyfcoding/pkg/logging"
)

// Step 表示 Saga 事务中的一个具体步骤。
type Step struct {
	Name       string
	Action     func(ctx context.Context) error
	Compensate func(ctx context.Context) error
	Retries    int
	Timeout    time.Duration
}

// Engine 负责管理并执行一系列步骤。
type Engine struct {
	steps []*Step
	mu    sync.Mutex
}

// NewEngine 创建并返回一个新的引擎。
func NewEngine() *Engine {
	return &Engine{
		steps: make([]*Step, 0),
	}
}

// AddStep 向引擎中增加一个事务步骤。
func (e *Engine) AddStep(name string, action, compensate func(ctx context.Context) error) *Engine {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.steps = append(e.steps, &Step{
		Name:       name,
		Action:     action,
		Compensate: compensate,
		Retries:    3,
		Timeout:    10 * time.Second,
	})
	return e
}

// Execute 执行整个 Saga 流程。
func (e *Engine) Execute(ctx context.Context) error {
	logger := logging.Default()
	executedSteps := make([]*Step, 0)

	for _, step := range e.steps {
		logger.InfoContext(ctx, "executing saga step", "step", step.Name)

		err := e.executeWithRetry(ctx, step)
		if err != nil {
			logger.ErrorContext(ctx, "saga step failed, starting compensation", "step", step.Name, "error", err)
			e.compensate(ctx, executedSteps)
			return fmt.Errorf("saga step %s failed: %w", step.Name, err)
		}

		executedSteps = append(executedSteps, step)
	}

	logger.InfoContext(ctx, "saga transaction completed successfully")
	return nil
}

func (e *Engine) executeWithRetry(ctx context.Context, step *Step) error {
	var lastErr error
	for i := 0; i < step.Retries; i++ {
		tCtx, cancel := context.WithTimeout(ctx, step.Timeout)
		lastErr = step.Action(tCtx)
		cancel()

		if lastErr == nil {
			return nil
		}

		if i < step.Retries-1 {
			time.Sleep(time.Duration(1<<uint(i)) * time.Second) // 指数退避
		}
	}
	return lastErr
}

func (e *Engine) compensate(ctx context.Context, steps []*Step) {
	logger := logging.Default()
	// 逆序回滚
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		if step.Compensate != nil {
			logger.WarnContext(ctx, "compensating step", "step", step.Name)
			if err := step.Compensate(ctx); err != nil {
				logger.ErrorContext(ctx, "compensation failed", "step", step.Name, "error", err)
				// 补偿失败通常需要人工干预或死信队列，此处记录关键日志
			}
		}
	}
}
