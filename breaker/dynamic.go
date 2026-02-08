// Package breaker 提供熔断器的动态更新能力。
package breaker

import (
	"sync/atomic"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/metrics"
)

// DynamicBreaker 提供支持热更新的熔断器封装。
type DynamicBreaker struct {
	value        atomic.Value
	name         string
	failureRatio float64
	minRequests  uint32
	metrics      *metrics.Metrics
}

// NewDynamicBreaker 创建动态熔断器。
func NewDynamicBreaker(name string, m *metrics.Metrics, failureRatio float64, minRequests uint32) *DynamicBreaker {
	return &DynamicBreaker{
		name:         name,
		failureRatio: failureRatio,
		minRequests:  minRequests,
		metrics:      m,
	}
}

// Update 根据最新配置重建熔断器。
func (d *DynamicBreaker) Update(cfg config.CircuitBreakerConfig) {
	if d == nil {
		return
	}
	if !cfg.Enabled {
		d.value.Store((*Breaker)(nil))
		return
	}

	d.value.Store(NewBreaker(Settings{
		Name:         d.name,
		Config:       cfg,
		FailureRatio: d.failureRatio,
		MinRequests:  d.minRequests,
	}, d.metrics))
}

// Execute 执行受熔断保护的函数。
func (d *DynamicBreaker) Execute(fn func() (any, error)) (any, error) {
	if d == nil {
		return fn()
	}
	inner := d.load()
	if inner == nil {
		return fn()
	}
	return inner.Execute(fn)
}

func (d *DynamicBreaker) load() *Breaker {
	if d == nil {
		return nil
	}
	v := d.value.Load()
	if v == nil {
		return nil
	}
	return v.(*Breaker)
}
