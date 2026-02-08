package metrics

import "github.com/prometheus/client_golang/prometheus"

// RegisterRequestSizeMetrics 注册请求体大小指标。
func (m *Metrics) RegisterRequestSizeMetrics() {
	if m == nil || m.HTTPRequestSizeBytes != nil {
		return
	}

	m.HTTPRequestSizeBytes = m.NewHistogramVec(&prometheus.HistogramOpts{
		Name:    "http_server_request_size_bytes",
		Help:    "HTTP request body size in bytes",
		Buckets: prometheus.ExponentialBuckets(128, 2, 8),
	}, []string{"method", "path"})
}
