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

// Metrics 核心结构体
type Metrics struct {
	registry *prometheus.Registry

	// 预定义的标准指标 (避免各处重复定义)
	HttpRequestsTotal    *prometheus.CounterVec
	HttpRequestDuration  *prometheus.HistogramVec
	GrpcRequestsTotal    *prometheus.CounterVec
	GrpcRequestDuration  *prometheus.HistogramVec
}

func NewMetrics(serviceName string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &Metrics{registry: reg}

	// 1. 初始化 HTTP 标准指标
	m.HttpRequestsTotal = m.NewCounterVec(prometheus.CounterOpts{
		Name: "http_server_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	m.HttpRequestDuration = m.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_server_request_duration_seconds",
		Help:    "HTTP request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	// 2. 初始化 gRPC 标准指标
	m.GrpcRequestsTotal = m.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_requests_total",
		Help: "Total number of gRPC requests",
	}, []string{"service", "method", "status"})

	m.GrpcRequestDuration = m.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "grpc_server_request_duration_seconds",
		Help:    "gRPC request latency in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method"})

	slog.Info("Unified metrics registry initialized", "service", serviceName)
	return m
}

func (m *Metrics) NewCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	cv := prometheus.NewCounterVec(opts, labelNames)
	m.registry.MustRegister(cv)
	return cv
}

func (m *Metrics) NewGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	gv := prometheus.NewGaugeVec(opts, labelNames)
	m.registry.MustRegister(gv)
	return gv
}

func (m *Metrics) NewHistogramVec(opts prometheus.HistogramOpts, labelNames []string) *prometheus.HistogramVec {
	hv := prometheus.NewHistogramVec(opts, labelNames)
	m.registry.MustRegister(hv)
	return hv
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

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
		ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
		srv.Shutdown(ctx)
	}
}