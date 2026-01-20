package transaction

import (
	"context"
	"fmt"
	"log/slog"
)

// SagaStep 定义了分布事务的一个步骤
type SagaStep interface {
	Name() string
	Execute(ctx context.Context) error
	Compensate(ctx context.Context) error
}

// SagaCoordinator 负责编排多个步骤的执行与补偿
type SagaCoordinator struct {
	steps []SagaStep
}

func NewSagaCoordinator() *SagaCoordinator {
	return &SagaCoordinator{
		steps: make([]SagaStep, 0),
	}
}

// AddStep 添加一个事务步骤
func (c *SagaCoordinator) AddStep(step SagaStep) *SagaCoordinator {
	c.steps = append(c.steps, step)
	return c
}

// Execute 按顺序执行所有步骤，若任一步骤失败则按相反顺序执行补偿
func (c *SagaCoordinator) Execute(ctx context.Context) error {
	executedSteps := make([]SagaStep, 0)

	for _, step := range c.steps {
		slog.Info("Executing Saga step", "step", step.Name())
		if err := step.Execute(ctx); err != nil {
			slog.Error("Saga step failed, starting compensation", "step", step.Name(), "error", err)
			c.compensate(ctx, executedSteps)
			return fmt.Errorf("saga failed at step [%s]: %w", step.Name(), err)
		}
		executedSteps = append(executedSteps, step)
	}

	slog.Info("Saga completed successfully")
	return nil
}

func (c *SagaCoordinator) compensate(ctx context.Context, executedSteps []SagaStep) {
	// 从最后一个执行成功的步骤开始反向补偿
	for i := len(executedSteps) - 1; i >= 0; i-- {
		step := executedSteps[i]
		slog.Info("Compensating Saga step", "step", step.Name())
		if err := step.Compensate(ctx); err != nil {
			slog.Error("Saga compensation failed", "step", step.Name(), "error", err)
			// 在实际生产中，补偿失败通常需要记入死信队列或触发人工介入
		}
	}
}

// BaseStep 提供基础实现辅助
type BaseStep struct {
	StepName string
}

func (b *BaseStep) Name() string {
	return b.StepName
}
