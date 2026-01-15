// Package fallback 提供了通用的业务降级处理机制。
package policy

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

var fallbackTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "service_fallback_total",
		Help: "Total number of service fallbacks executed",
	},
	[]string{"service", "operation"},
)

func init() {
	prometheus.MustRegister(fallbackTotal)
}

// Func 定义了可执行的业务函数原型。
type Func[T any] func(ctx context.Context) (T, error)

// ExecuteWithFallback 执行带降级保护的业务逻辑。
func ExecuteWithFallback[T any](
	ctx context.Context,
	service, operation string,
	mainFunc Func[T],
	fallbackFunc Func[T],
) (T, error) {
	// 1. 执行主逻辑。
	res, err := mainFunc(ctx)
	if err == nil {
		return res, nil
	}

	// 2. 发生错误，记录日志并执行降级。
	slog.WarnContext(ctx, "main logic failed, executing fallback",
		"service", service,
		"operation", operation,
		"error", err,
	)

	// 3. 增加监控指标。
	fallbackTotal.WithLabelValues(service, operation).Inc()

	return fallbackFunc(ctx)
}
