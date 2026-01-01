package breaker

import (
	"errors"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/metrics"
)

var ErrServiceUnavailable = errors.New("service unavailable (circuit breaker open)")

// Breaker 封装了具备指标监控能力的熔断器
type Breaker struct {
	cb      *gobreaker.CircuitBreaker
	metrics *prometheus.GaugeVec
}

// Settings 熔断器配置
type Settings struct {
	Name   string
	Config config.CircuitBreakerConfig
	// 失败率阈值 (0.0 - 1.0)，默认 0.5
	FailureRatio float64
	// 触发熔断所需的最小请求数，默认 10
	MinRequests uint32
}

// NewBreaker 创建一个新的熔断器，并自动注册监控指标
func NewBreaker(st Settings, m *metrics.Metrics) *Breaker {
	// 如果配置未启用，返回一个永远不熔断的“伪熔断器” (gobreaker 不支持直接禁用，我们通过逻辑控制或默认设置)
	if !st.Config.Enabled {
		st.Config.MaxRequests = 0 // 0 means no limit in gobreaker for max requests
	}

	if st.FailureRatio <= 0 {
		st.FailureRatio = 0.5
	}
	if st.MinRequests <= 0 {
		st.MinRequests = 10
	}

	// 注册指标 (1: Closed, 2: Open, 3: Half-Open)
	cbStatus := m.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_state",
		Help: "Circuit breaker state (1:Closed, 2:Open, 3:HalfOpen)",
	}, []string{"name"})

	readyToTrip := func(counts gobreaker.Counts) bool {
		// 如果禁用了熔断，永远不触发
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
		slog.Warn("Circuit breaker state changed", "name", name, "from", from.String(), "to", to.String())
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

// Execute 执行受熔断保护的逻辑
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
