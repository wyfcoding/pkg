// Package adaptive 自适应限流器
// 基于系统负载（CPU/Latency）动态调整限流阈值
package adaptive

import (
	"context"
	"math"
	"time"

	"github.com/wyfcoding/pkg/limiter"
)

// StatsProvider 提供系统负载指标
type StatsProvider interface {
	GetCPUUsage() float64 // 0.0 - 1.0
	GetAvgLatency() time.Duration
	GetInFlight() int64
}

// Config 配置
type Config struct {
	MinRate       float64 // 最小 QPS
	MaxRate       float64 // 最大 QPS
	TargetCPU     float64 // 目标 CPU 使用率 (e.g. 0.8)
	CheckInterval time.Duration
}

// AdaptiveLimiter 自适应限流器
type AdaptiveLimiter struct {
	limiter     limiter.Limiter // 底层令牌桶
	stats       StatsProvider
	config      Config
	currentRate float64
	checkTicker *time.Ticker
	stopChan    chan struct{}
}

// NewAdaptiveLimiter 创建自适应限流器
func NewAdaptiveLimiter(base limiter.Limiter, stats StatsProvider, cfg Config) *AdaptiveLimiter {
	l := &AdaptiveLimiter{
		limiter:     base,
		stats:       stats,
		config:      cfg,
		currentRate: cfg.MaxRate, // 初始使用最大值或最小值
		checkTicker: time.NewTicker(cfg.CheckInterval),
		stopChan:    make(chan struct{}),
	}
	go l.loop()
	return l
}

func (l *AdaptiveLimiter) Allow(ctx context.Context, key string) (bool, error) {
	return l.limiter.Allow(ctx, key)
}

func (l *AdaptiveLimiter) loop() {
	for {
		select {
		case <-l.checkTicker.C:
			l.adjust()
		case <-l.stopChan:
			l.checkTicker.Stop()
			return
		}
	}
}

// adjust 根据负载调整限流阈值 (简化版 TCP BBR 思想)
func (l *AdaptiveLimiter) adjust() {
	cpu := l.stats.GetCPUUsage()

	// 如果 CPU 超过目标，降低限流
	if cpu > l.config.TargetCPU {
		// 降低 10% 或按比例
		factor := l.config.TargetCPU / cpu
		l.currentRate = math.Max(l.config.MinRate, l.currentRate*factor)
	} else {
		// CPU 较低，尝试缓慢增加，探测上限
		l.currentRate = math.Min(l.config.MaxRate, l.currentRate+10.0) // 线性增加
	}

	// 更新底层限流器 (假设 Limiter 有 SetRate 方法，需 interface 支持)
	// l.limiter.SetRate(l.currentRate)
	// 注意：当前 pkg/limiter 接口可能不支持动态 SetRate，这需要修改 limiter 包。
	// 这里仅演示逻辑。
}

func (l *AdaptiveLimiter) Close() {
	close(l.stopChan)
}
