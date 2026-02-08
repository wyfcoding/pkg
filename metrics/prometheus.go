// Package metrics 提供了基于 Prometheus 的应用指标采集能力。
// 生成摘要:
// 1) 增加 HTTP/gRPC 在途请求指标。
// 2) 增加 HTTP/gRPC 慢请求计数指标。
// 假设:
// 1) 在途请求以 method+path / service+method 作为维度标签。
package metrics

import (
	"context"
	"errors"
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

	// BuildInfo 版本构建信息指标。
	BuildInfo *prometheus.GaugeVec
	// HTTPRequestsTotal HTTP 请求总量 (维度: method, path, status)。
	HTTPRequestsTotal *prometheus.CounterVec
	// HTTPRequestDuration HTTP 请求耗时分布。
	HTTPRequestDuration *prometheus.HistogramVec
	// HTTPRequestSizeBytes HTTP 请求体大小分布。
	HTTPRequestSizeBytes *prometheus.HistogramVec
	// HTTPResponseSizeBytes HTTP 响应体大小分布。
	HTTPResponseSizeBytes *prometheus.HistogramVec
	// HTTPInFlight HTTP 当前在途请求数。
	HTTPInFlight *prometheus.GaugeVec
	// HTTPSlowRequestsTotal HTTP 慢请求总量。
	HTTPSlowRequestsTotal *prometheus.CounterVec
	// HTTPRateLimitTotal HTTP 限流触发次数。
	HTTPRateLimitTotal *prometheus.CounterVec
	// GRPCRequestsTotal gRPC 请求总量 (维度: service, method, status)。
	GRPCRequestsTotal *prometheus.CounterVec
	// GRPCRequestDuration gRPC 请求耗时分布。
	GRPCRequestDuration *prometheus.HistogramVec
	// GRPCInFlight gRPC 当前在途请求数。
	GRPCInFlight *prometheus.GaugeVec
	// GRPCSlowRequestsTotal gRPC 慢请求总量。
	GRPCSlowRequestsTotal *prometheus.CounterVec
	// GRPCRateLimitTotal gRPC 限流触发次数。
	GRPCRateLimitTotal *prometheus.CounterVec
}

// NewMetrics 初始化并返回一个新的指标采集器。
func NewMetrics(serviceName string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &Metrics{registry: reg}

	m.HTTPRequestsTotal = m.NewCounterVec(&prometheus.CounterOpts{
		Name: "http_server_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	m.HTTPRequestDuration = m.NewHistogramVec(&prometheus.HistogramOpts{
		Name:    "http_server_request_duration_seconds",
		Help:    "HTTP request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	m.HTTPInFlight = m.NewGaugeVec(&prometheus.GaugeOpts{
		Name: "http_server_in_flight_requests",
		Help: "Current number of HTTP in-flight requests",
	}, []string{"method", "path"})

	m.HTTPSlowRequestsTotal = m.NewCounterVec(&prometheus.CounterOpts{
		Name: "http_server_slow_requests_total",
		Help: "Total number of slow HTTP requests",
	}, []string{"method", "path"})

	m.HTTPRateLimitTotal = m.NewCounterVec(&prometheus.CounterOpts{
		Name: "http_server_rate_limit_total",
		Help: "Total number of rate limited HTTP requests",
	}, []string{"method", "path"})

	m.GRPCRequestsTotal = m.NewCounterVec(&prometheus.CounterOpts{
		Name: "grpc_server_requests_total",
		Help: "Total number of gRPC requests",
	}, []string{"service", "method", "status"})

	m.GRPCRequestDuration = m.NewHistogramVec(&prometheus.HistogramOpts{
		Name:    "grpc_server_request_duration_seconds",
		Help:    "gRPC request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method"})

	m.GRPCInFlight = m.NewGaugeVec(&prometheus.GaugeOpts{
		Name: "grpc_server_in_flight_requests",
		Help: "Current number of gRPC in-flight requests",
	}, []string{"service", "method"})

	m.GRPCSlowRequestsTotal = m.NewCounterVec(&prometheus.CounterOpts{
		Name: "grpc_server_slow_requests_total",
		Help: "Total number of slow gRPC requests",
	}, []string{"service", "method"})

	m.GRPCRateLimitTotal = m.NewCounterVec(&prometheus.CounterOpts{
		Name: "grpc_server_rate_limit_total",
		Help: "Total number of rate limited gRPC requests",
	}, []string{"service", "method"})

	m.RegisterRequestSizeMetrics()
	m.RegisterResponseSizeMetrics()

	slog.Info("unified metrics registry initialized", "service", serviceName)

	return m
}

// Register 注册自定义指标。
func (m *Metrics) Register(collectors ...prometheus.Collector) {
	m.registry.MustRegister(collectors...)
}

// NewCounterVec 创建并注册一个新的计数器指标。
func (m *Metrics) NewCounterVec(opts *prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	cv := prometheus.NewCounterVec(*opts, labelNames)
	m.registry.MustRegister(cv)
	return cv
}

// NewGauge 创建并注册一个新的仪表盘指标。
func (m *Metrics) NewGauge(opts *prometheus.GaugeOpts) prometheus.Gauge {
	g := prometheus.NewGauge(*opts)
	m.registry.MustRegister(g)
	return g
}

// NewGaugeVec 创建并注册一个新的仪表盘指标。
func (m *Metrics) NewGaugeVec(opts *prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	gv := prometheus.NewGaugeVec(*opts, labelNames)
	m.registry.MustRegister(gv)
	return gv
}

// NewHistogramVec 创建并注册一个新的直方图指标。
func (m *Metrics) NewHistogramVec(opts *prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec {
	hv := prometheus.NewHistogramVec(*opts, labelNames)
	m.registry.MustRegister(hv)
	return hv
}

// NewGaugeFunc 创建并注册一个新的函数式仪表盘指标。
func (m *Metrics) NewGaugeFunc(opts *prometheus.GaugeOpts, function func() float64) prometheus.GaugeFunc {
	gf := prometheus.NewGaugeFunc(*opts, function)
	m.registry.MustRegister(gf)
	return gf
}

// Handler 返回用于暴露指标的 HTTP 处理器。
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ExposeHTTP 在指定端口启动一个独立的 HTTP 服务器用于暴露指标数据。
func (m *Metrics) ExposeHTTP(port string) (stop func()) {
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           m.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
