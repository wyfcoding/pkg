// Package retry 提供了工业级的指数退避重试机制，支持随机抖动（Jitter）以防止大规模重试引发的惊群效应。
package retry

import (
	"context"
	"log/slog"
	"math/rand"
	"time"
)

// RetryFunc 定义了可被重试执行的业务函数原型。
type RetryFunc func() error

// RetryConfig 封装了重试策略的详细控制参数。
type RetryConfig struct {
	MaxRetries     int           // 最大重试次数（不含首次尝试）
	InitialBackoff time.Duration // 首次重试前的初始等待时间
	MaxBackoff     time.Duration // 重试等待时间的上限（封顶值）
	Multiplier     float64       // 等待时间随重试次数增长的乘数（指数因子）
	Jitter         float64       // 随机抖动因子 (0.0-1.0)，用于分散重试压力
}

// ... (DefaultRetryConfig 保持不变) ...

// Retry 根据配置的策略执行函数 fn，并在发生错误时进行自动重试。
// DefaultRetryConfig 返回一个默认的、通用的重试配置。
// 默认配置为：最大重试3次，初始等待100毫秒，最长等待2秒，时间倍率为2.0，抖动为10%。
// 这个配置适用于大多数常规的、对延迟不极端敏感的场景。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		Multiplier:     2.0,
		Jitter:         0.1,
	}
}

// 核心逻辑：执行 -> 失败 -> 计算退避时间 -> 等待（支持上下文取消）-> 重试。
func Retry(ctx context.Context, fn RetryFunc, cfg RetryConfig) error {
	var err error
	backoff := cfg.InitialBackoff

	for i := 0; i <= cfg.MaxRetries; i++ {
		if err = fn(); err == nil {
			return nil // 执行成功，立即返回
		}

		if i == cfg.MaxRetries {
			slog.WarnContext(ctx, "maximum retries reached", "retries", i, "error", err)
			break
		}

		slog.WarnContext(ctx, "retry attempt failed", "attempt", i+1, "next_backoff", backoff, "error", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		// 计算下一次指数退避时间
		nextBackoff := float64(backoff) * cfg.Multiplier

		// 注入随机抖动，防止惊群效应
		if cfg.Jitter > 0 {
			jitterValue := (rand.Float64()*2 - 1) * cfg.Jitter * nextBackoff
			nextBackoff += jitterValue
		}

		backoff = min(time.Duration(nextBackoff), cfg.MaxBackoff)
	}

	return err
}
