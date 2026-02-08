package metrics

import "github.com/prometheus/client_golang/prometheus"

// RegisterResponseSizeMetrics 注册响应体大小指标。
func (m *Metrics) RegisterResponseSizeMetrics() {
	if m == nil || m.HTTPResponseSizeBytes != nil {
		return
	}

	m.HTTPResponseSizeBytes = m.NewHistogramVec(&prometheus.HistogramOpts{
		Name:    "http_server_response_size_bytes",
		Help:    "HTTP response body size in bytes",
		Buckets: prometheus.ExponentialBuckets(128, 2, 8),
	}, []string{"method", "path"})
}
