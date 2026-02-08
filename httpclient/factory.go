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
	serviceName := resolveServiceName(logger)

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
		Rules:          convertRules(cfg.Rules),
	}, logger, metricsInstance)
}

// UpdateFromConfig 基于最新配置更新 HTTP 客户端治理策略。
func (c *Client) UpdateFromConfig(cfg config.HTTPClientConfig) {
	if c == nil {
		return
	}
	retryCfg := normalizeRetryConfig(cfg)
	serviceName := resolveServiceName(c.logger)
	c.UpdateConfig(Config{
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
		Rules:          convertRules(cfg.Rules),
	})
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

func convertRules(rules []config.HTTPClientRule) []Rule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, Rule{
			Name:            rule.Name,
			Hosts:           rule.Hosts,
			Methods:         rule.Methods,
			Paths:           rule.Paths,
			Timeout:         rule.Timeout,
			RateLimit:       rule.RateLimit,
			RateBurst:       rule.RateBurst,
			MaxConcurrency:  rule.MaxConcurrency,
			SlowThreshold:   rule.SlowThreshold,
			RetryMax:        rule.RetryMax,
			RetryInitial:    rule.RetryInitial,
			RetryMaxBackoff: rule.RetryMaxBackoff,
			RetryMultiplier: rule.RetryMultiplier,
			RetryJitter:     rule.RetryJitter,
			RetryStatus:     rule.RetryStatus,
			RetryMethods:    rule.RetryMethods,
			Breaker:         rule.Breaker,
		})
	}
	return out
}

func resolveServiceName(logger *logging.Logger) string {
	serviceName := "httpclient"
	if logger != nil && logger.Service != "" {
		serviceName = logger.Service
	}
	return serviceName
}
