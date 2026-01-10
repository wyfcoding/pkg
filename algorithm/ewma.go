package algorithm

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrInvalidAlpha = errors.New("alpha must be between 0 and 1")
)

// EWMA (Exponentially Weighted Moving Average) 指数加权移动平均。
// 它能平滑时间序列数据，并对近期的数据点给予更高的权重。
// 适用场景：负载均衡（Latency 敏感）、动态定价（价格平滑）、监控指标告警。
type EWMA struct {
	mu    sync.RWMutex
	alpha float64 // 平滑系数 (0 < alpha < 1)，值越大对新数据越灵敏。
	value float64 // 当前的平均值。
	init  bool    // 是否已初始化。
}

// NewEWMA 创建一个新的 EWMA 实例。
// alpha 通常取 2/(N+1)，其中 N 是你想要平均的数据点周期。
// 例如：N=10, alpha=0.18。
func NewEWMA(alpha float64) (*EWMA, error) {
	if alpha <= 0 || alpha >= 1 {
		return nil, ErrInvalidAlpha
	}
	return &EWMA{
		alpha: alpha,
	}, nil
}

// Update 更新平均值。
func (e *EWMA) Update(newValue float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.init {
		e.value = newValue
		e.init = true
		return
	}

	e.value = e.alpha*newValue + (1-e.alpha)*e.value
}

// Value 获取当前平均值。
func (e *EWMA) Value() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.value
}

// MovingLatency 专门用于统计请求延迟的 EWMA。
type MovingLatency struct {
	ewma *EWMA
}

func NewMovingLatency(alpha float64) (*MovingLatency, error) {
	ewma, err := NewEWMA(alpha)
	if err != nil {
		return nil, err
	}
	return &MovingLatency{
		ewma: ewma,
	}, nil
}

// Observe 观测一次耗时。
func (ml *MovingLatency) Observe(d time.Duration) {
	ml.ewma.Update(float64(d.Milliseconds()))
}

// LatencyMS 获取当前估算的延迟（毫秒）。
func (ml *MovingLatency) LatencyMS() float64 {
	return ml.ewma.Value()
}
