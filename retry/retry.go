// Package retry 提供了工业级的指数退避重试机制.
package retry

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

// Func 定义了可被重试执行的业务函数原型.
type Func func() error

// Config 封装了重试策略的详细控制参数.
type Config struct { // 重试策略配置，已对齐。
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Jitter         float64
	MaxRetries     int
}

// DefaultRetryConfig 返回一个通用的默认重试配置.
func DefaultRetryConfig() Config {
	return Config{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		Multiplier:     2.0,
		Jitter:         0.1,
	}
}

// Retry 根据配置的策略执行函数 fn.
func Retry(ctx context.Context, fn Func, cfg Config) error {
	return RetryIf(ctx, fn, func(error) bool { return true }, cfg)
}

// RetryIf 仅在 shouldRetry 返回 true 时进行重试.
func RetryIf(ctx context.Context, fn Func, shouldRetry func(error) bool, cfg Config) error {
	if cfg.MaxRetries < 0 {
		return fn()
	}

	var lastErr error
	backoff := cfg.InitialBackoff

	for retryIdx := 0; retryIdx <= cfg.MaxRetries; retryIdx++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if retryIdx == cfg.MaxRetries || !shouldRetry(lastErr) {
			break
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-time.After(backoff):
		}

		nextBackoff := float64(backoff) * cfg.Multiplier

		if cfg.Jitter > 0 {
			rv := rand.Float64()
			jitterValue := (rv*2 - 1) * cfg.Jitter * nextBackoff
			nextBackoff += jitterValue
		}

		backoff = min(time.Duration(nextBackoff), cfg.MaxBackoff)
	}

	return fmt.Errorf("retry failed after %d attempts: %w", cfg.MaxRetries, lastErr)
}
