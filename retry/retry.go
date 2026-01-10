// Package retry 提供了工业级的指数退避重试机制。
package retry

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	randv2 "math/rand/v2"
	"time"
)

// Func 定义了可被重试执行的业务函数原型。
type Func func() error

// Config 封装了重试策略的详细控制参数。
type Config struct {
	InitialBackoff time.Duration // 首次重试前的初始等待时间。
	MaxBackoff     time.Duration // 重试等待时间的上限（封顶值）。
	Multiplier     float64       // 等待时间随重试次数增长的乘数（指数因子）。
	Jitter         float64       // 随机抖动因子 (0.0-1.0)。
	MaxRetries     int           // 最大重试次数（不含首次尝试）。
}

// DefaultRetryConfig 返回一个通用的默认重试配置。
func DefaultRetryConfig() Config {
	return Config{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
		Multiplier:     2.0,
		Jitter:         0.1,
	}
}

// Retry 根据配置的策略执行函数 fn，并在发生错误时进行自动重试。
func Retry(ctx context.Context, fn Func, cfg Config) error {
	var lastErr error
	backoff := cfg.InitialBackoff

	var seed [8]byte
	if _, readErr := rand.Read(seed[:]); readErr != nil {
		// 极低概率失败，退化为时间戳。
		binary.LittleEndian.PutUint64(seed[:], uint64(time.Now().UnixNano()))
	}

	randomSrc := randv2.New(randv2.NewPCG(binary.LittleEndian.Uint64(seed[:]), 0))

	for retryIdx := 0; retryIdx <= cfg.MaxRetries; retryIdx++ {
		lastErr = fn()
		if lastErr == nil {
			return nil // 执行成功，立即返回。
		}

		if retryIdx == cfg.MaxRetries {
			break
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-time.After(backoff):
		}

		// 计算下一次指数退避时间。
		nextBackoff := float64(backoff) * cfg.Multiplier

		// 注入随机抖动，防止惊群效应。
		if cfg.Jitter > 0 {
			jitterValue := (randomSrc.Float64()*2 - 1) * cfg.Jitter * nextBackoff
			nextBackoff += jitterValue
		}

		backoff = time.Duration(nextBackoff)
		if backoff > cfg.MaxBackoff {
			backoff = cfg.MaxBackoff
		}
	}

	return fmt.Errorf("maximum retries (%d) reached: %w", cfg.MaxRetries, lastErr)
}
