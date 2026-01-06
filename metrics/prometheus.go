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
	registry *prometheus.Registry // 内部独立的 Prometheus 注册中心

	// 预定义的标准指标，减少各业务模块的样板代码
	HttpRequestsTotal   *prometheus.CounterVec   // HTTP 请求总量 (维度: method, path, status)
	HttpRequestDuration *prometheus.HistogramVec // HTTP 请求耗时分布
	GrpcRequestsTotal   *prometheus.CounterVec   // gRPC 请求总量 (维度: service, method, status)
	GrpcRequestDuration *prometheus.HistogramVec // gRPC 请求耗时分布
}

// NewMetrics 初始化并返回一个新的指标采集器。
// 它会自动注册 Go 运行时指标和进程指标。
func NewMetrics(serviceName string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &Metrics{registry: reg}

	// 初始化各标准指标...
	m.HttpRequestsTotal = m.NewCounterVec(prometheus.CounterOpts{
		Name: "http_server_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	m.HttpRequestDuration = m.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_server_request_duration_seconds",
		Help:    "HTTP request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	m.GrpcRequestsTotal = m.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_requests_total",
		Help: "Total number of gRPC requests",
	}, []string{"service", "method", "status"})

	m.GrpcRequestDuration = m.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "grpc_server_request_duration_seconds",
		Help:    "gRPC request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method"})

	slog.Info("unified metrics registry initialized", "service", serviceName)
	return m
}

// NewCounterVec 创建并注册一个新的计数器指标。
func (m *Metrics) NewCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	cv := prometheus.NewCounterVec(opts, labelNames)
	m.registry.MustRegister(cv)
	return cv
}

// NewGaugeVec 创建并注册一个新的仪表盘指标。
func (m *Metrics) NewGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	gv := prometheus.NewGaugeVec(opts, labelNames)
	m.registry.MustRegister(gv)
	return gv
}

// NewHistogramVec 创建并注册一个新的直方图指标。
func (m *Metrics) NewHistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec {
	hv := prometheus.NewHistogramVec(opts, labelNames)
	m.registry.MustRegister(hv)
	return hv
}

// Handler 返回用于暴露指标的 HTTP 处理器。
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ExposeHttp 在指定端口启动一个独立的 HTTP 服务器用于暴露指标数据。
// 返回一个清理函数用于优雅关闭该服务器。
func (m *Metrics) ExposeHttp(port string) func() {
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: m.Handler(),
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "error", err)
		}
	}()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("failed to shutdown metrics server", "error", err)
		}
	}
}
