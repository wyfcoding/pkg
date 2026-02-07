// Package limiter 提供了限流器的通用接口与多种后端实现。
// 生成摘要:
// 1) 新增并发信号量限流器，支持阻塞获取与快速失败。
// 2) 提供标准错误，便于中间件统一处理过载场景。
// 假设:
// 1) Release 与 Acquire 成对使用，超额释放仅记录告警。
package limiter

import (
	"context"
	"errors"
	"log/slog"
)

// ErrConcurrencyLimit 表示并发上限已触发。
var ErrConcurrencyLimit = errors.New("concurrency limit exceeded")

// ConcurrencyLimiter 定义并发控制的通用接口。
type ConcurrencyLimiter interface {
	Acquire(ctx context.Context) error
	TryAcquire() bool
	Release()
}

// SemaphoreLimiter 使用带缓冲的信号量实现并发控制。
type SemaphoreLimiter struct {
	sem      chan struct{}
	disabled bool
}

// NewSemaphoreLimiter 创建一个并发信号量限流器。
// max <= 0 表示禁用并发限制。
func NewSemaphoreLimiter(max int) *SemaphoreLimiter {
	if max <= 0 {
		return &SemaphoreLimiter{disabled: true}
	}

	return &SemaphoreLimiter{sem: make(chan struct{}, max)}
}

// Acquire 获取一个并发令牌，支持 Context 取消。
func (l *SemaphoreLimiter) Acquire(ctx context.Context) error {
	if l == nil || l.disabled {
		return nil
	}

	select {
	case l.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryAcquire 尝试获取一个并发令牌，快速失败。
func (l *SemaphoreLimiter) TryAcquire() bool {
	if l == nil || l.disabled {
		return true
	}

	select {
	case l.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release 释放一个并发令牌。
func (l *SemaphoreLimiter) Release() {
	if l == nil || l.disabled {
		return
	}

	select {
	case <-l.sem:
		return
	default:
		slog.Warn("concurrency limiter release without acquire")
	}
}
