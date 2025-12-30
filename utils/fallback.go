package utils

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

// fallbackTotal 记录降级发生的次数
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

// FallbackFunc 定义了要执行的业务函数
type FallbackFunc[T any] func(ctx context.Context) (T, error)

// ExecuteWithFallback 执行带降级的逻辑
// service: 服务名，operation: 操作名（用于监控）
// mainFunc: 主业务逻辑
// fallbackFunc: 降级逻辑（兜底）
func ExecuteWithFallback[T any](
	ctx context.Context,
	service, operation string,
	mainFunc FallbackFunc[T],
	fallbackFunc FallbackFunc[T],
) (T, error) {
	// 1. 执行主逻辑
	res, err := mainFunc(ctx)
	if err == nil {
		return res, nil
	}

	// 2. 发生错误，记录日志并执行降级
	slog.WarnContext(ctx, "main logic failed, executing fallback",
		"service", service,
		"operation", operation,
		"error", err,
	)

	// 3. 增加监控指标
	fallbackTotal.WithLabelValues(service, operation).Inc()

	return fallbackFunc(ctx)
}
