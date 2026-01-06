// Package breaker 提供了基于 gobreaker 的熔断器封装，集成了 Prometheus 指标监控。
package breaker

import (
	"errors"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/metrics"
)

// ErrServiceUnavailable 当熔断器处于开启状态时返回此错误。
var ErrServiceUnavailable = errors.New("service unavailable (circuit breaker open)")

// Breaker 封装了具备指标监控能力的熔断器实例。
type Breaker struct {
	cb      *gobreaker.CircuitBreaker // 底层 gobreaker 实例
	metrics *prometheus.GaugeVec      // 关联的 Prometheus 指标
}

// Settings 定义了熔断器的核心运行参数。
type Settings struct {
	Name         string                      // 熔断器唯一标识名称
	Config       config.CircuitBreakerConfig // 基础配置（超时、间隔等）
	FailureRatio float64                     // 失败率阈值 (0.0 - 1.0)，默认 0.5
	MinRequests  uint32                      // 触发熔断判断所需的最小请求数
}

// NewBreaker 初始化并返回一个带有自动监控指标的熔断器。
func NewBreaker(st Settings, m *metrics.Metrics) *Breaker {
	// 如果配置未启用，返回一个永远不熔断的“伪熔断器”
	if !st.Config.Enabled {
		st.Config.MaxRequests = 0
	}

	if st.FailureRatio <= 0 {
		st.FailureRatio = 0.5
	}
	if st.MinRequests <= 0 {
		st.MinRequests = 10
	}

	// 注册状态指标 (1: Closed, 2: Open, 3: Half-Open)
	cbStatus := m.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_state",
		Help: "Circuit breaker state (1:Closed, 2:Open, 3:HalfOpen)",
	}, []string{"name"})

	readyToTrip := func(counts gobreaker.Counts) bool {
		if !st.Config.Enabled {
			return false
		}
		ratio := float64(counts.TotalFailures) / float64(counts.Requests)
		return counts.Requests >= st.MinRequests && ratio >= st.FailureRatio
	}

	onStateChange := func(name string, from, to gobreaker.State) {
		val := 1.0
		switch to {
		case gobreaker.StateOpen:
			val = 2.0
		case gobreaker.StateHalfOpen:
			val = 3.0
		}
		cbStatus.WithLabelValues(name).Set(val)
		slog.Warn("circuit breaker state changed", "name", name, "from", from.String(), "to", to.String())
	}

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:          st.Name,
		MaxRequests:   st.Config.MaxRequests,
		Interval:      st.Config.Interval,
		Timeout:       st.Config.Timeout,
		ReadyToTrip:   readyToTrip,
		OnStateChange: onStateChange,
	})

	cbStatus.WithLabelValues(st.Name).Set(1.0)

	return &Breaker{
		cb:      cb,
		metrics: cbStatus,
	}
}

// Execute 执行受熔断保护的业务逻辑。
// 如果熔断器处于开启状态，立即返回 ErrServiceUnavailable。
func (b *Breaker) Execute(req func() (any, error)) (any, error) {
	result, err := b.cb.Execute(req)
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, ErrServiceUnavailable
		}
		return nil, err
	}
	return result, nil
}
