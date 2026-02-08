package httpclient

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/retry"
	"golang.org/x/time/rate"
)

// Rule 定义 HTTP 客户端的动态治理规则。
type Rule struct {
	Name            string
	Hosts           []string
	Methods         []string
	Paths           []string
	Timeout         *time.Duration
	RateLimit       *int
	RateBurst       *int
	MaxConcurrency  *int
	SlowThreshold   *time.Duration
	RetryMax        *int
	RetryInitial    *time.Duration
	RetryMaxBackoff *time.Duration
	RetryMultiplier *float64
	RetryJitter     *float64
	RetryStatus     []int
	RetryMethods    []string
	Breaker         *config.CircuitBreakerConfig
}

type breakerExecutor interface {
	Execute(fn func() (any, error)) (any, error)
}

type clientPolicy struct {
	timeout       time.Duration
	limiter       limiter.Limiter
	concurrency   limiter.ConcurrencyLimiter
	breaker       breakerExecutor
	slowThreshold time.Duration
	retryConfig   retry.Config
	retryStatus   map[int]struct{}
	retryMethods  map[string]struct{}
}

type policyRule struct {
	hosts   []string
	methods []string
	paths   []string
	policy  *clientPolicy
}

type policyStore struct {
	defaultPolicy *clientPolicy
	rules         []policyRule
}

// UpdateConfig 更新客户端治理配置。
func (c *Client) UpdateConfig(cfg Config) {
	if c == nil {
		return
	}
	store := buildPolicyStore(cfg, c.metricsInstance)
	c.policies.Store(store)
}

func (c *Client) matchPolicy(req *http.Request) *clientPolicy {
	store := c.loadPolicyStore()
	if store == nil {
		return nil
	}
	return store.match(req)
}

func (c *Client) loadPolicyStore() *policyStore {
	if c == nil {
		return nil
	}
	value := c.policies.Load()
	if value == nil {
		return nil
	}
	return value.(*policyStore)
}

func buildPolicyStore(cfg Config, metricsInstance *metrics.Metrics) *policyStore {
	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "httpclient"
	}

	baseTimeout := resolveTimeout(cfg.Timeout)
	basePolicy := buildPolicy(policyConfig{
		name:           breakerName(serviceName, "", true, 0),
		timeout:        baseTimeout,
		rateLimit:      cfg.RateLimit,
		rateBurst:      cfg.RateBurst,
		maxConcurrency: cfg.MaxConcurrency,
		slowThreshold:  cfg.SlowThreshold,
		breaker:        cfg.BreakerConfig,
		retryConfig:    cfg.RetryConfig,
		retryStatus:    cfg.RetryStatus,
		retryMethods:   cfg.RetryMethods,
	}, metricsInstance)

	store := &policyStore{
		defaultPolicy: basePolicy,
	}

	if len(cfg.Rules) == 0 {
		return store
	}

	store.rules = make([]policyRule, 0, len(cfg.Rules))
	for idx, rule := range cfg.Rules {
		timeout := baseTimeout
		if rule.Timeout != nil {
			timeout = *rule.Timeout
		}

		rateLimit := cfg.RateLimit
		if rule.RateLimit != nil {
			rateLimit = *rule.RateLimit
		}
		rateBurst := cfg.RateBurst
		if rule.RateBurst != nil {
			rateBurst = *rule.RateBurst
		}

		maxConcurrency := cfg.MaxConcurrency
		if rule.MaxConcurrency != nil {
			maxConcurrency = *rule.MaxConcurrency
		}

		slowThreshold := cfg.SlowThreshold
		if rule.SlowThreshold != nil {
			slowThreshold = *rule.SlowThreshold
		}

		breakerCfg := cfg.BreakerConfig
		if rule.Breaker != nil {
			breakerCfg = *rule.Breaker
		}

		retryCfg := normalizeRetryConfigFromRule(rule, cfg.RetryConfig)
		retryStatus := cfg.RetryStatus
		if len(rule.RetryStatus) > 0 {
			retryStatus = rule.RetryStatus
		}
		retryMethods := cfg.RetryMethods
		if len(rule.RetryMethods) > 0 {
			retryMethods = rule.RetryMethods
		}

		policy := buildPolicy(policyConfig{
			name:           breakerName(serviceName, rule.Name, false, idx),
			timeout:        timeout,
			rateLimit:      rateLimit,
			rateBurst:      rateBurst,
			maxConcurrency: maxConcurrency,
			slowThreshold:  slowThreshold,
			breaker:        breakerCfg,
			retryConfig:    retryCfg,
			retryStatus:    retryStatus,
			retryMethods:   retryMethods,
		}, metricsInstance)

		store.rules = append(store.rules, policyRule{
			hosts:   normalizePatterns(rule.Hosts, strings.ToLower),
			methods: normalizePatterns(rule.Methods, strings.ToUpper),
			paths:   normalizePatterns(rule.Paths, nil),
			policy:  policy,
		})
	}

	return store
}

func (s *policyStore) match(req *http.Request) *clientPolicy {
	if s == nil {
		return nil
	}
	for _, rule := range s.rules {
		if rule.matches(req) {
			return rule.policy
		}
	}
	return s.defaultPolicy
}

func (r policyRule) matches(req *http.Request) bool {
	if req == nil {
		return false
	}
	host := strings.ToLower(hostKey(req))
	if !matchAny(host, r.hosts) {
		return false
	}
	method := strings.ToUpper(req.Method)
	if !matchAny(method, r.methods) {
		return false
	}
	path := ""
	if req.URL != nil {
		path = req.URL.Path
	}
	if !matchAny(path, r.paths) {
		return false
	}
	return true
}

type policyConfig struct {
	name           string
	timeout        time.Duration
	rateLimit      int
	rateBurst      int
	maxConcurrency int
	slowThreshold  time.Duration
	breaker        config.CircuitBreakerConfig
	retryConfig    retry.Config
	retryStatus    []int
	retryMethods   []string
}

func buildPolicy(cfg policyConfig, metricsInstance *metrics.Metrics) *clientPolicy {
	return &clientPolicy{
		timeout:       cfg.timeout,
		limiter:       buildLimiter(cfg.rateLimit, cfg.rateBurst),
		concurrency:   buildConcurrency(cfg.maxConcurrency),
		breaker:       buildBreaker(cfg.name, cfg.breaker, metricsInstance),
		slowThreshold: cfg.slowThreshold,
		retryConfig:   cfg.retryConfig,
		retryStatus:   normalizeRetryStatus(cfg.retryStatus),
		retryMethods:  normalizeRetryMethods(cfg.retryMethods),
	}
}

func buildLimiter(rateLimit, burst int) limiter.Limiter {
	if rateLimit <= 0 {
		return nil
	}
	if burst <= 0 {
		burst = rateLimit
	}
	return limiter.NewLocalLimiter(rate.Limit(rateLimit), burst)
}

func buildConcurrency(max int) limiter.ConcurrencyLimiter {
	if max <= 0 {
		return nil
	}
	return limiter.NewSemaphoreLimiter(max)
}

func buildBreaker(name string, cfg config.CircuitBreakerConfig, metricsInstance *metrics.Metrics) breakerExecutor {
	if !cfg.Enabled {
		return nil
	}
	dynamicBreaker := breaker.NewDynamicBreaker(name, metricsInstance, 0, 0)
	dynamicBreaker.Update(cfg)
	return dynamicBreaker
}

func breakerName(serviceName, ruleName string, isDefault bool, idx int) string {
	if isDefault {
		return fmt.Sprintf("http-client-%s", serviceName)
	}
	if ruleName != "" {
		return fmt.Sprintf("http-client-%s-%s", serviceName, ruleName)
	}
	return fmt.Sprintf("http-client-%s-rule-%d", serviceName, idx+1)
}

func resolveTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultTimeout
	}
	return timeout
}

func normalizeRetryConfigFromRule(rule Rule, base retry.Config) retry.Config {
	hasOverride := rule.RetryMax != nil || rule.RetryInitial != nil || rule.RetryMaxBackoff != nil || rule.RetryMultiplier != nil || rule.RetryJitter != nil
	cfg := base
	if hasOverride && retryConfigEmpty(base) {
		cfg = retry.DefaultRetryConfig()
	}
	if rule.RetryMax != nil {
		cfg.MaxRetries = *rule.RetryMax
	}
	if rule.RetryInitial != nil {
		cfg.InitialBackoff = *rule.RetryInitial
	}
	if rule.RetryMaxBackoff != nil {
		cfg.MaxBackoff = *rule.RetryMaxBackoff
	}
	if rule.RetryMultiplier != nil {
		cfg.Multiplier = *rule.RetryMultiplier
	}
	if rule.RetryJitter != nil {
		cfg.Jitter = *rule.RetryJitter
	}
	return cfg
}

func retryConfigEmpty(cfg retry.Config) bool {
	return cfg.MaxRetries == 0 && cfg.InitialBackoff == 0 && cfg.MaxBackoff == 0 && cfg.Multiplier == 0 && cfg.Jitter == 0
}

func normalizePatterns(patterns []string, normalize func(string) string) []string {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if normalize != nil {
			pattern = normalize(pattern)
		}
		out = append(out, pattern)
	}
	return out
}

func matchAny(value string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if matchPattern(value, pattern) {
			return true
		}
	}
	return false
}

func matchPattern(value, pattern string) bool {
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return value == pattern
	}

	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(pattern, "*") {
		if len(parts) == 0 || !strings.HasPrefix(value, parts[0]) {
			return false
		}
	}
	if !strings.HasSuffix(pattern, "*") {
		last := parts[len(parts)-1]
		if last != "" && !strings.HasSuffix(value, last) {
			return false
		}
	}

	searchStart := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(value[searchStart:], part)
		if idx < 0 {
			return false
		}
		searchStart += idx + len(part)
	}
	return true
}

func (p *clientPolicy) shouldRetryStatus(code int) bool {
	if p == nil {
		return false
	}
	if len(p.retryStatus) == 0 {
		return false
	}
	_, ok := p.retryStatus[code]
	return ok
}

func (p *clientPolicy) methodRetryAllowed(method string) bool {
	if p == nil {
		return false
	}
	if len(p.retryMethods) == 0 {
		return false
	}
	_, ok := p.retryMethods[strings.ToUpper(method)]
	return ok
}
