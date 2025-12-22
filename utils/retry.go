// Package utils 提供了通用的工具函数集合。
// 此文件实现了带指数退避和抖动的重试逻辑，用于提高系统在面对瞬时故障时的容错能力。
package utils

import (
	"context"
	"math/rand"
	"time"
)

// RetryFunc 是一个可重试的函数类型。
// 它封装了可能需要重试的业务逻辑操作。
type RetryFunc func() error

// RetryConfig 定义了重试策略的详细配置。
// 这些参数共同决定了重试行为的频率、次数和时间间隔。
type RetryConfig struct {
	// MaxRetries 是最大重试次数。当达到此次数后，如果操作仍失败，将返回最后一次的错误。
	MaxRetries int
	// InitialBackoff 是初始的退避（等待）时间。第一次重试前将等待此时间。
	InitialBackoff time.Duration
	// MaxBackoff 是最长的退避时间。退避时间会随重试次数指数增长，但不会超过此上限。
	MaxBackoff time.Duration
	// Multiplier 是每次重试后，退避时间的增长因子。例如，2.0表示每次退避时间翻倍。
	Multiplier float64
	// Jitter 是一个抖动因子，用于在退避时间上增加一个随机变化量（正负均可）。
	// 例如，0.1 表示在计算出的退避时间基础上，增加或减少最多10%的随机时间。
	// 抖动可以有效避免多个实例在同一时刻集体发起重试，从而减少“惊群效应”。
	Jitter float64
}

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

// Retry 根据指定的配置（cfg）来执行一个函数（fn），并在其返回错误时进行重试。
// 它结合了指数退避（Exponential Backoff）和抖动（Jitter）策略。
//
// 工作流程:
// 1. 立即执行一次 `fn`。如果成功（返回 nil），则函数直接返回 nil。
// 2. 如果 `fn` 失败，则进入重试循环，最多重试 `MaxRetries` 次。
// 3. 在每次重试前，函数会检查 `context` 是否已被取消。如果已取消，则立即返回 `context.Err()`。
// 4. 如果未取消，则根据当前退避时间 `backoff` 进行等待。
// 5. 等待结束后，计算下一次的退避时间：`backoff = backoff * Multiplier ± Jitter`。
// 6. 新的 `backoff` 不会超过 `MaxBackoff`。
// 7. 循环执行 `fn`，直到成功或达到最大重试次数。
// 8. 如果所有重试都失败，则返回最后一次 `fn` 执行时遇到的错误。
func Retry(ctx context.Context, fn RetryFunc, cfg RetryConfig) error {
	var err error
	backoff := cfg.InitialBackoff

	for i := 0; i <= cfg.MaxRetries; i++ {
		// 执行业务函数
		if err = fn(); err == nil {
			// executed successfully，直接返回
			return nil
		}

		// 如果已达到最大重试次数，则不再继续，跳出循环并返回最后一次的错误
		if i == cfg.MaxRetries {
			break
		}

		// 在两次重试之间，检查上下文是否被取消
		select {
		case <-ctx.Done():
			// 如果上下文被取消（例如，由于超时或父级请求终止），则返回上下文错误
			return ctx.Err()
		case <-time.After(backoff):
			// 等待指定的退避时间
		}

		// --- 计算下一次退避时间 ---
		// 指数增长
		nextBackoff := float64(backoff) * cfg.Multiplier

		// 增加抖动，避免惊群
		if cfg.Jitter > 0 {
			// 计算抖动值：一个在 [-Jitter * nextBackoff, Jitter * nextBackoff] 范围内的随机数
			jitterValue := (rand.Float64()*2 - 1) * cfg.Jitter * nextBackoff
			nextBackoff += jitterValue
		}

		backoff = min(
			// 确保退避时间不超过设定的最大值
			time.Duration(nextBackoff), cfg.MaxBackoff)
	}

	// 返回最后一次尝试的错误
	return err
}
