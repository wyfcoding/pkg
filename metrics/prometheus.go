// Package metrics 提供了基于 Prometheus 的应用指标采集能力。
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

// Metrics 封装了基于 Prometheus 的指标采集注册表及预定义的标准监控指标。
type Metrics struct {
	registry *prometheus.Registry

	// HTTPRequestsTotal HTTP 请求总量 (维度: method, path, status)。
	HTTPRequestsTotal *prometheus.CounterVec
	// HTTPRequestDuration HTTP 请求耗时分布。
	HTTPRequestDuration *prometheus.HistogramVec
	// GRPCRequestsTotal gRPC 请求总量 (维度: service, method, status)。
	GRPCRequestsTotal *prometheus.CounterVec
	// GRPCRequestDuration gRPC 请求耗时分布。
	GRPCRequestDuration *prometheus.HistogramVec
}

// NewMetrics 初始化并返回一个新的指标采集器。
// 它会自动注册 Go 运行时指标和进程指标。
func NewMetrics(serviceName string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	metricsInstance := &Metrics{registry: reg}

	metricsInstance.HTTPRequestsTotal = metricsInstance.NewCounterVec(prometheus.CounterOpts{
		Name: "http_server_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	metricsInstance.HTTPRequestDuration = metricsInstance.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_server_request_duration_seconds",
		Help:    "HTTP request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	metricsInstance.GRPCRequestsTotal = metricsInstance.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_requests_total",
		Help: "Total number of gRPC requests",
	}, []string{"service", "method", "status"})

	metricsInstance.GRPCRequestDuration = metricsInstance.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "grpc_server_request_duration_seconds",
		Help:    "gRPC request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method"})

	slog.Info("unified metrics registry initialized", "service", serviceName)

	return metricsInstance
}

// NewCounterVec 创建并注册一个新的计数器指标。
func (m *Metrics) NewCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	counterVec := prometheus.NewCounterVec(opts, labelNames)
	m.registry.MustRegister(counterVec)

	return counterVec
}

// NewGaugeVec 创建并注册一个新的仪表盘指标。
func (m *Metrics) NewGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	gaugeVec := prometheus.NewGaugeVec(opts, labelNames)
	m.registry.MustRegister(gaugeVec)

	return gaugeVec
}

// NewHistogramVec 创建并注册一个新的直方图指标。
func (m *Metrics) NewHistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec {
	histogramVec := prometheus.NewHistogramVec(opts, labelNames)
	m.registry.MustRegister(histogramVec)

	return histogramVec
}

// Handler 返回用于暴露指标的 HTTP 处理器。
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ExposeHTTP 在指定端口启动一个独立的 HTTP 服务器用于暴露指标数据。
// 返回一个清理函数用于优雅关闭该服务器。
func (m *Metrics) ExposeHTTP(port string) func() {
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           m.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()

	return func() {
		const shutdownTimeout = 5 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			slog.Error("failed to shutdown metrics server", "error", err)
		}
	}
}