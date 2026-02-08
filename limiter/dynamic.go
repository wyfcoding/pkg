// Package limiter 提供限流器的动态更新能力。
package limiter

import (
	"context"
	"sync/atomic"

	"golang.org/x/time/rate"
)

// DynamicLimiter 提供支持热更新的限流器封装。
type DynamicLimiter struct {
	value atomic.Value
}

// NewDynamicLimiter 创建动态限流器。
func NewDynamicLimiter(initial Limiter) *DynamicLimiter {
	d := &DynamicLimiter{}
	if initial != nil {
		d.value.Store(initial)
	}
	return d
}

// NewDynamicLocalLimiter 创建基于本地令牌桶的动态限流器。
func NewDynamicLocalLimiter(rateLimit, burst int) *DynamicLimiter {
	d := NewDynamicLimiter(nil)
	d.UpdateLocal(rateLimit, burst)
	return d
}

// Update 替换当前限流器实例。
func (d *DynamicLimiter) Update(l Limiter) {
	if d == nil {
		return
	}
	d.value.Store(l)
}

// UpdateLocal 更新为本地令牌桶限流器。
func (d *DynamicLimiter) UpdateLocal(rateLimit, burst int) {
	if d == nil {
		return
	}
	if rateLimit <= 0 {
		d.Update(nil)
		return
	}
	if burst <= 0 {
		burst = rateLimit
	}
	d.Update(NewLocalLimiter(rate.Limit(rateLimit), burst))
}

// Allow 实现 Limiter 接口。
func (d *DynamicLimiter) Allow(ctx context.Context, key string) (bool, error) {
	if d == nil {
		return true, nil
	}
	limiter := d.load()
	if limiter == nil {
		return true, nil
	}
	return limiter.Allow(ctx, key)
}

func (d *DynamicLimiter) load() Limiter {
	if d == nil {
		return nil
	}
	v := d.value.Load()
	if v == nil {
		return nil
	}
	return v.(Limiter)
}

// DynamicConcurrencyLimiter 提供支持热更新的并发限流器。
type DynamicConcurrencyLimiter struct {
	value atomic.Value
}

// NewDynamicConcurrencyLimiter 创建动态并发限流器。
func NewDynamicConcurrencyLimiter(initial ConcurrencyLimiter) *DynamicConcurrencyLimiter {
	d := &DynamicConcurrencyLimiter{}
	if initial != nil {
		d.value.Store(initial)
	}
	return d
}

// NewDynamicSemaphoreLimiter 创建基于信号量的动态并发限流器。
func NewDynamicSemaphoreLimiter(max int) *DynamicConcurrencyLimiter {
	d := NewDynamicConcurrencyLimiter(nil)
	d.UpdateSemaphore(max)
	return d
}

// Update 替换当前并发限流器实例。
func (d *DynamicConcurrencyLimiter) Update(l ConcurrencyLimiter) {
	if d == nil {
		return
	}
	d.value.Store(l)
}

// UpdateSemaphore 更新为信号量并发限流器。
func (d *DynamicConcurrencyLimiter) UpdateSemaphore(max int) {
	if d == nil {
		return
	}
	if max <= 0 {
		d.Update(nil)
		return
	}
	d.Update(NewSemaphoreLimiter(max))
}

// Acquire 获取并发令牌。
func (d *DynamicConcurrencyLimiter) Acquire(ctx context.Context) error {
	limiter := d.load()
	if limiter == nil {
		return nil
	}
	return limiter.Acquire(ctx)
}

// TryAcquire 尝试获取并发令牌。
func (d *DynamicConcurrencyLimiter) TryAcquire() bool {
	limiter := d.load()
	if limiter == nil {
		return true
	}
	return limiter.TryAcquire()
}

// Release 释放并发令牌。
func (d *DynamicConcurrencyLimiter) Release() {
	limiter := d.load()
	if limiter == nil {
		return
	}
	limiter.Release()
}

func (d *DynamicConcurrencyLimiter) load() ConcurrencyLimiter {
	if d == nil {
		return nil
	}
	v := d.value.Load()
	if v == nil {
		return nil
	}
	return v.(ConcurrencyLimiter)
}
