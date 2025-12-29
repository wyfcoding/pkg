// Package metrics 提供了Prometheus指标的收集和暴露功能。
// 它封装了Prometheus的注册表，并提供了创建Counter、Gauge、Histogram等指标的便捷方法，
// 同时提供了一个HTTP服务用于暴露这些指标，供Prometheus服务器抓取。
package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics 结构体封装了一个Prometheus注册表（registry），
// 用于管理服务中所有的自定义和默认指标。
type Metrics struct {
	registry *prometheus.Registry
}

// NewMetrics 创建一个带有自定义Prometheus注册表的新Metrics实例。
// serviceName: 服务的名称，用于日志记录。
// 在创建时，会自动注册Go运行时指标和进程相关指标。
func NewMetrics(serviceName string) *Metrics {
	registry := prometheus.NewRegistry()
	// 注册Go运行时指标，例如goroutine数量、内存使用等。
	registry.MustRegister(collectors.NewGoCollector())
	// 注册进程相关指标，例如CPU使用率、文件描述符数量等。
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	slog.Info("Metrics registry created for service", "service", serviceName)

	return &Metrics{registry: registry}
}

// NewCounterVec 创建、注册并返回一个新的CounterVec（计数器向量）。
// CounterVec是一种带标签的计数器，用于统计事件发生的次数。
func (m *Metrics) NewCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	counter := prometheus.NewCounterVec(opts, labelNames)
	m.registry.MustRegister(counter) // 将计数器注册到Metrics实例的注册表中。
	return counter
}

// NewGaugeVec 创建、注册并返回一个新的GaugeVec（仪表向量）。
// GaugeVec是一种带标签的仪表，用于测量当前值，例如队列长度、CPU利用率。
func (m *Metrics) NewGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	gauge := prometheus.NewGaugeVec(opts, labelNames)
	m.registry.MustRegister(gauge) // 将仪表注册到Metrics实例的注册表中。
	return gauge
}

// NewHistogramVec 创建、注册并返回一个新的HistogramVec（直方图向量）。
// HistogramVec用于统计样本的分布情况，例如请求延迟、响应大小。
func (m *Metrics) NewHistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec {
	histogram := prometheus.NewHistogramVec(opts, labelNames)
	m.registry.MustRegister(histogram) // 将直方图注册到Metrics实例的注册表中。
	return histogram
}

// Handler 返回一个 http.Handler，用于暴露注册表中的指标。
// 它可以直接集成到 Gin 或标准库的 HTTP 路由中。
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ExposeHttp 启动一个HTTP服务器，用于在指定的端口暴露Prometheus指标。
// PromQL查询可以通过访问 `/metrics` 路径获取这些指标。
// port: 服务器监听的端口号。
// 返回一个清理函数，该函数用于优雅地关闭HTTP服务器。
func (m *Metrics) ExposeHttp(port string) func() {
	httpServer := &http.Server{
		Addr:    ":" + port,                                              // 监听所有网卡的指定端口。
		Handler: promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}), // 使用Prometheus处理器暴露指标。
	}

	// 在一个新的goroutine中启动HTTP服务器，以避免阻塞主线程。
	go func() {
		slog.Info("Metrics server listening", "port", port)
		// ListenAndServe会阻塞，直到服务器关闭或发生错误。
		// http.ErrServerClosed 是一个预期的错误，表示服务器已正常关闭。
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to start metrics server", "error", err)
		}
	}()

	// 返回一个清理函数，用于在应用退出时优雅地关闭指标服务器。
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // 设置关闭超时。
		defer cancel()
		slog.Info("shutting down metrics server...")
		if err := httpServer.Shutdown(ctx); err != nil {
			slog.Error("failed to gracefully shutdown metrics server", "error", err)
		}
	}

	return cleanup
}
