package httpclient

import (
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/retry"
)

// NewFromConfig 基于统一配置构造 HTTP 客户端。
func NewFromConfig(cfg config.HTTPClientConfig, logger *logging.Logger, metricsInstance *metrics.Metrics) *Client {
	retryCfg := normalizeRetryConfig(cfg)
	serviceName := "httpclient"
	if logger != nil && logger.Service != "" {
		serviceName = logger.Service
	}

	return NewClient(Config{
		ServiceName:    serviceName,
		Timeout:        cfg.Timeout,
		RateLimit:      cfg.RateLimit,
		RateBurst:      cfg.RateBurst,
		MaxConcurrency: cfg.MaxConcurrency,
		SlowThreshold:  cfg.SlowThreshold,
		BreakerConfig:  cfg.Breaker,
		RetryConfig:    retryCfg,
		RetryStatus:    cfg.RetryStatus,
		RetryMethods:   cfg.RetryMethods,
	}, logger, metricsInstance)
}

func normalizeRetryConfig(cfg config.HTTPClientConfig) retry.Config {
	if cfg.RetryMax == 0 && cfg.RetryInitial == 0 && cfg.RetryMaxBackoff == 0 && cfg.RetryMultiplier == 0 && cfg.RetryJitter == 0 {
		return retry.Config{}
	}

	base := retry.DefaultRetryConfig()
	if cfg.RetryMax != 0 {
		base.MaxRetries = cfg.RetryMax
	}
	if cfg.RetryInitial != 0 {
		base.InitialBackoff = cfg.RetryInitial
	}
	if cfg.RetryMaxBackoff != 0 {
		base.MaxBackoff = cfg.RetryMaxBackoff
	}
	if cfg.RetryMultiplier != 0 {
		base.Multiplier = cfg.RetryMultiplier
	}
	if cfg.RetryJitter != 0 {
		base.Jitter = cfg.RetryJitter
	}

	return base
}
