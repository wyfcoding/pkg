package metrics

import "github.com/prometheus/client_golang/prometheus"

// RegisterBuildInfo 注册构建信息指标。
func (m *Metrics) RegisterBuildInfo(serviceName, version string) {
	if m == nil || m.BuildInfo != nil {
		return
	}
	if serviceName == "" {
		serviceName = "unknown"
	}
	if version == "" {
		version = "unknown"
	}

	m.BuildInfo = m.NewGaugeVec(&prometheus.GaugeOpts{
		Name: "build_info",
		Help: "Build information for the service",
	}, []string{"service", "version"})

	m.BuildInfo.WithLabelValues(serviceName, version).Set(1)
}
