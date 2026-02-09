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

// Orchestrator 负责管理并执行一系列步骤。
type Orchestrator struct {
	steps []*Step
	mu    sync.Mutex
}

// NewOrchestrator 创建并返回一个新的编排器。
func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		steps: make([]*Step, 0),
	}
}

// AddStep 向编排器中增加一个事务步骤。
func (o *Orchestrator) AddStep(name string, action, compensate func(ctx context.Context) error) *Orchestrator {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.steps = append(o.steps, &Step{
		Name:       name,
		Action:     action,
		Compensate: compensate,
		Retries:    3,
		Timeout:    10 * time.Second,
	})
	return o
}

// Execute 执行整个 Saga 流程。
func (o *Orchestrator) Execute(ctx context.Context) error {
	logger := logging.Default()
	executedSteps := make([]*Step, 0)

	for _, step := range o.steps {
		logger.InfoContext(ctx, "executing saga step", "step", step.Name)

		err := o.executeWithRetry(ctx, step)
		if err != nil {
			logger.ErrorContext(ctx, "saga step failed, starting compensation", "step", step.Name, "error", err)
			o.compensate(ctx, executedSteps)
			return fmt.Errorf("saga step %s failed: %w", step.Name, err)
		}

		executedSteps = append(executedSteps, step)
	}

	logger.InfoContext(ctx, "saga transaction completed successfully")
	return nil
}

func (o *Orchestrator) executeWithRetry(ctx context.Context, step *Step) error {
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

func (o *Orchestrator) compensate(ctx context.Context, steps []*Step) {
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
