// Package breaker 提供了基于 gobreaker 的增强型熔断器实现。
package breaker

import (
	"errors"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/metrics"
)

// ErrServiceUnavailable 表示服务当前处于熔断状态。
var ErrServiceUnavailable = errors.New("service unavailable: circuit breaker is open")

// Breaker 封装了 gobreaker 实例，集成了 Prometheus 指标监控与日志。
type Breaker struct {
	circuitBreaker *gobreaker.CircuitBreaker
	metrics        *prometheus.GaugeVec
}

// Settings 定义了熔断器的初始化参数。
type Settings struct {
	Name         string
	Config       config.CircuitBreakerConfig
	FailureRatio float64
	MinRequests  uint32
}

// NewBreaker 初始化并返回一个新的熔断器封装对象。
func NewBreaker(st Settings, m *metrics.Metrics) *Breaker {
	if !st.Config.Enabled {
		return &Breaker{circuitBreaker: nil}
	}

	failureRatio := st.FailureRatio
	if failureRatio <= 0 {
		failureRatio = 0.5
	}

	minRequests := st.MinRequests
	if minRequests == 0 {
		minRequests = 5
	}

	var metricsVec *prometheus.GaugeVec
	if m != nil {
		metricsVec = m.NewGaugeVec(&prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "Circuit breaker state (1: Closed, 2: Open, 3: Half-Open)",
		}, []string{"name"})
	}

	gs := gobreaker.Settings{
		Name:        st.Name,
		MaxRequests: st.Config.MaxRequests,
		Interval:    st.Config.Interval,
		Timeout:     st.Config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			ratio := float64(counts.TotalFailures) / float64(counts.Requests)

			return counts.Requests >= minRequests && ratio >= failureRatio
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("circuit breaker state changed",
				"name", name,
				"from", from.String(),
				"to", to.String(),
			)
			if metricsVec != nil {
				metricsVec.WithLabelValues(name).Set(float64(to))
			}
		},
	}

	cb := gobreaker.NewCircuitBreaker(gs)

	return &Breaker{
		circuitBreaker: cb,
		metrics:        metricsVec,
	}
}

// Execute 执行受熔断保护的函数。
func (b *Breaker) Execute(fn func() (any, error)) (any, error) {
	if b.circuitBreaker == nil {
		return fn()
	}

	res, err := b.circuitBreaker.Execute(fn)
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, ErrServiceUnavailable
		}

		return nil, err
	}

	return res, nil
}

// ExecuteTyped 是 Execute 的泛型版本，提供更好的类型安全。
func ExecuteTyped[T any](b *Breaker, fn func() (T, error)) (T, error) {
	if b == nil || b.circuitBreaker == nil {
		return fn()
	}

	res, err := b.circuitBreaker.Execute(func() (any, error) {
		return fn()
	})
	if err != nil {
		var zero T
		if errors.Is(err, gobreaker.ErrOpenState) {
			return zero, ErrServiceUnavailable
		}
		return zero, err
	}

	return res.(T), nil
}
