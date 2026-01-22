// Package policy 提供了统一的执行策略抽象，结合了重试、熔断与回退机制.
package policy

import (
	"context"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/retry"
)

// Policy 定义了通用的执行策略接口.
type Policy interface {
	Execute(ctx context.Context, fn func() error) error
}

// DefaultPolicy 组合了熔断与重试的默认策略.
type DefaultPolicy struct {
	breaker  *breaker.Breaker
	retryCfg retry.Config
}

// NewDefaultPolicy 创建一个新的组合策略.
func NewDefaultPolicy(b *breaker.Breaker, r retry.Config) *DefaultPolicy {
	return &DefaultPolicy{
		breaker:  b,
		retryCfg: r,
	}
}

// Execute 执行带熔断和重试保护的任务.
func (p *DefaultPolicy) Execute(ctx context.Context, fn func() error) error {
	return retry.Retry(ctx, func() error {
		if p.breaker == nil {
			return fn()
		}
		_, err := p.breaker.Execute(func() (any, error) {
			return nil, fn()
		})
		return err
	}, p.retryCfg)
}

// ExecuteTyped 带泛型的执行方法.
func ExecuteTyped[T any](ctx context.Context, p Policy, fn func() (T, error)) (T, error) {
	var result T
	err := p.Execute(ctx, func() error {
		var innerErr error
		result, innerErr = fn()
		return innerErr
	})
	return result, err
}
