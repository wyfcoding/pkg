// Package fallback 提供了通用的业务降级处理机制，支持 Prometheus 指标监控与结构化日志。
package fallback

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

// fallbackTotal 记录全系统内降级操作触发的次数计数器。
// 维度包含：service (所属微服务), operation (具体操作名称)。
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

// Func 定义了可执行的业务函数原型，支持泛型返回结果。
type Func[T any] func(ctx context.Context) (T, error)

// ExecuteWithFallback 执行带降级保护的业务逻辑。
// 参数说明：
//   - service: 发起降级的服务标识
//   - operation: 执行的操作名称（用于监控区分）
//   - mainFunc: 优先执行的主业务逻辑
//   - fallbackFunc: 主逻辑失败后的兜底逻辑
//
// 返回：主逻辑结果或降级后的结果，以及可能的错误。
func ExecuteWithFallback[T any](
	ctx context.Context,
	service, operation string,
	mainFunc Func[T],
	fallbackFunc Func[T],
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
