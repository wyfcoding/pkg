package grpcclient

import (
	"sync/atomic"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
)

var defaultFactory atomic.Value

// SetDefaultFactory 设置全局默认的 gRPC 客户端工厂。
func SetDefaultFactory(factory *ClientFactory) {
	if factory == nil {
		return
	}
	defaultFactory.Store(factory)
}

// DefaultFactory 返回全局默认的 gRPC 客户端工厂。
func DefaultFactory() *ClientFactory {
	val := defaultFactory.Load()
	if val == nil {
		return nil
	}
	return val.(*ClientFactory)
}

func pickFactory(metricsInstance *metrics.Metrics, cbCfg config.CircuitBreakerConfig) *ClientFactory {
	if factory := DefaultFactory(); factory != nil {
		if cbCfg != (config.CircuitBreakerConfig{}) {
			factory.UpdateCircuitBreaker(cbCfg)
		}
		return factory
	}
	return NewClientFactory(logging.Default(), metricsInstance, cbCfg)
}
