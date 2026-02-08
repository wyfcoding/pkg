// Package httpclient 提供具备治理能力的 HTTP 客户端封装。
// 生成摘要:
// 1) 增加统一的限流、熔断、重试、慢请求监控与链路注入能力。
// 2) 自动透传 Request/Trace/租户/用户/权限/角色等关键头信息。
// 假设:
// 1) 仅对幂等方法启用重试，非幂等请求需显式关闭重试。
package httpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/idgen"
	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/middleware"
	"github.com/wyfcoding/pkg/retry"
	"github.com/wyfcoding/pkg/tracing"
	"golang.org/x/time/rate"
)

var (
	// ErrRateLimit 表示触发 HTTP 客户端限流。
	ErrRateLimit = errors.New("http client rate limit exceeded")
	// ErrConcurrencyLimit 表示触发 HTTP 客户端并发限制。
	ErrConcurrencyLimit = errors.New("http client concurrency limit exceeded")
	// ErrRequestBodyNotReplayable 表示请求体不可重复读取，无法重试。
	ErrRequestBodyNotReplayable = errors.New("request body is not replayable")
)

const (
	defaultTimeout = 10 * time.Second
)

// Config 定义 HTTP 客户端的治理配置。
type Config struct {
	ServiceName    string
	Timeout        time.Duration
	BreakerConfig  config.CircuitBreakerConfig
	RateLimit      int
	RateBurst      int
	MaxConcurrency int
	SlowThreshold  time.Duration
	RetryConfig    retry.Config
	RetryStatus    []int
	RetryMethods   []string
}

// Client 封装标准 http.Client，提供治理能力。
type Client struct {
	client          *http.Client
	logger          *logging.Logger
	breaker         *breaker.Breaker
	limiter         limiter.Limiter
	concurrency     limiter.ConcurrencyLimiter
	slowThreshold   time.Duration
	retryConfig     retry.Config
	retryStatus     map[int]struct{}
	retryMethods    map[string]struct{}
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	slowRequests    *prometheus.CounterVec
}

// NewClient 创建一个带治理能力的 HTTP 客户端。
func NewClient(cfg Config, logger *logging.Logger, metricsInstance *metrics.Metrics) *Client {
	if metricsInstance == nil {
		metricsInstance = metrics.NewMetrics(cfg.ServiceName)
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	retryCfg := cfg.RetryConfig
	if retryCfg == (retry.Config{}) {
		retryCfg = retry.DefaultRetryConfig()
	}

	retryMethods := normalizeRetryMethods(cfg.RetryMethods)
	retryStatus := normalizeRetryStatus(cfg.RetryStatus)

	limit := limiter.NewLocalLimiter(rate.Limit(maxInt(cfg.RateLimit, 0)), maxInt(cfg.RateBurst, 0))
	if cfg.RateLimit <= 0 {
		limit = limiter.NewLocalLimiter(rate.Limit(2000), 200)
	}

	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "http-client-" + cfg.ServiceName,
		Config: cfg.BreakerConfig,
	}, metricsInstance)

	requestsTotal := metricsInstance.NewCounterVec(&prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "http_client",
		Name:      "requests_total",
		Help:      "HTTP client request count",
	}, []string{"host", "method", "status"})

	requestDuration := metricsInstance.NewHistogramVec(&prometheus.HistogramOpts{
		Namespace: "pkg",
		Subsystem: "http_client",
		Name:      "request_duration_seconds",
		Help:      "HTTP client request latency",
		Buckets:   prometheus.DefBuckets,
	}, []string{"host", "method"})

	slowRequests := metricsInstance.NewCounterVec(&prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "http_client",
		Name:      "slow_requests_total",
		Help:      "HTTP client slow request count",
	}, []string{"host", "method"})

	return &Client{
		client: &http.Client{
			Timeout: timeout,
		},
		logger:          logger,
		breaker:         cb,
		limiter:         limit,
		concurrency:     limiter.NewSemaphoreLimiter(cfg.MaxConcurrency),
		slowThreshold:   cfg.SlowThreshold,
		retryConfig:     retryCfg,
		retryStatus:     retryStatus,
		retryMethods:    retryMethods,
		requestsTotal:   requestsTotal,
		requestDuration: requestDuration,
		slowRequests:    slowRequests,
	}
}

// Do 发起 HTTP 请求并返回响应。
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}

	ctx = ensureContext(ctx, req)
	injectHeaders(req, ctx)
	injectTraceContext(req, ctx)

	if err := c.concurrency.Acquire(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConcurrencyLimit, err)
	}
	defer c.concurrency.Release()

	allowed, err := c.limiter.Allow(ctx, "http:"+hostKey(req))
	if err != nil {
		c.logWarn(ctx, "http client limiter error", "error", err)
	}
	if !allowed {
		return nil, ErrRateLimit
	}

	if !c.methodRetryAllowed(req.Method) {
		return c.doOnce(ctx, req)
	}

	var lastResp *http.Response
	err = retry.If(ctx, func() error {
		reqClone, cloneErr := cloneRequest(ctx, req)
		if cloneErr != nil {
			return cloneErr
		}

		resp, callErr := c.doOnce(ctx, reqClone)
		if callErr != nil {
			lastResp = nil
			return callErr
		}

		if c.shouldRetryStatus(resp.StatusCode) {
			lastResp = resp
			drainAndClose(resp.Body)
			return retryableStatusError{code: resp.StatusCode}
		}

		lastResp = resp
		return nil
	}, c.shouldRetry, c.retryConfig)

	if err != nil {
		if _, ok := err.(retryableStatusError); ok && lastResp != nil {
			return lastResp, nil
		}
		if errors.Is(err, ErrRequestBodyNotReplayable) && lastResp != nil {
			return lastResp, nil
		}
		return nil, err
	}

	return lastResp, nil
}

func (c *Client) doOnce(ctx context.Context, req *http.Request) (*http.Response, error) {
	start := time.Now()
	var resp *http.Response

	_, err := c.breaker.Execute(func() (any, error) {
		spanCtx, span := tracing.Tracer().Start(ctx, "HTTPClient."+strings.ToUpper(req.Method))
		defer span.End()

		tracing.AddTag(spanCtx, "http.method", req.Method)
		tracing.AddTag(spanCtx, "http.url", req.URL.String())
		tracing.AddTag(spanCtx, "http.host", hostKey(req))

		response, callErr := c.client.Do(req.WithContext(spanCtx))
		resp = response
		if callErr != nil {
			tracing.SetError(spanCtx, callErr)
			return nil, callErr
		}
		return nil, nil
	})

	duration := time.Since(start)
	status := "error"
	if resp != nil {
		status = fmt.Sprintf("%d", resp.StatusCode)
	}

	c.record(req, status, duration)
	c.checkSlow(ctx, req, duration)

	return resp, err
}

func (c *Client) record(req *http.Request, status string, duration time.Duration) {
	if c.requestsTotal != nil {
		c.requestsTotal.WithLabelValues(hostKey(req), req.Method, status).Inc()
	}
	if c.requestDuration != nil {
		c.requestDuration.WithLabelValues(hostKey(req), req.Method).Observe(duration.Seconds())
	}
}

func (c *Client) checkSlow(ctx context.Context, req *http.Request, duration time.Duration) {
	if c.slowThreshold <= 0 || duration < c.slowThreshold {
		return
	}
	if c.slowRequests != nil {
		c.slowRequests.WithLabelValues(hostKey(req), req.Method).Inc()
	}
	c.logWarn(ctx, "http client slow request", "method", req.Method, "url", req.URL.String(), "duration", duration)
}

func (c *Client) shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRequestBodyNotReplayable) {
		return false
	}
	_, ok := err.(retryableStatusError)
	return ok || !errors.Is(err, breaker.ErrServiceUnavailable)
}

func (c *Client) shouldRetryStatus(code int) bool {
	_, ok := c.retryStatus[code]
	return ok
}

func (c *Client) methodRetryAllowed(method string) bool {
	_, ok := c.retryMethods[strings.ToUpper(method)]
	return ok
}

func (c *Client) logWarn(ctx context.Context, msg string, args ...any) {
	if c.logger != nil {
		c.logger.WarnContext(ctx, msg, args...)
		return
	}
	logging.Warn(ctx, msg, args...)
}

func ensureContext(ctx context.Context, req *http.Request) context.Context {
	if ctx != nil {
		return ctx
	}
	return req.Context()
}

func injectHeaders(req *http.Request, ctx context.Context) {
	setHeaderIfEmpty(req, middleware.HeaderXRequestID, requestID(ctx))
	setHeaderIfEmpty(req, middleware.HeaderXTraceID, tracing.GetTraceID(ctx))
	setHeaderIfEmpty(req, middleware.HeaderXTenantID, contextx.GetTenantID(ctx))
	setHeaderIfEmpty(req, middleware.HeaderXUserID, contextx.GetUserID(ctx))
	setHeaderIfEmpty(req, middleware.HeaderXScopes, contextx.GetScopes(ctx))
	setHeaderIfEmpty(req, middleware.HeaderXRole, contextx.GetRole(ctx))
}

func injectTraceContext(req *http.Request, ctx context.Context) {
	carrier := tracing.InjectContext(ctx)
	for k, v := range carrier {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
}

func requestID(ctx context.Context) string {
	val := contextx.GetRequestID(ctx)
	if val != "" {
		return val
	}
	return idgen.GenIDString()
}

func setHeaderIfEmpty(req *http.Request, key, value string) {
	if value == "" {
		return
	}
	if req.Header.Get(key) == "" {
		req.Header.Set(key, value)
	}
}

func hostKey(req *http.Request) string {
	if req == nil || req.URL == nil {
		return "unknown"
	}
	if req.Host != "" {
		return req.Host
	}
	return req.URL.Host
}

func cloneRequest(ctx context.Context, req *http.Request) (*http.Request, error) {
	if req.GetBody == nil && req.Body != nil {
		return nil, ErrRequestBodyNotReplayable
	}

	clone := req.Clone(ctx)
	if req.Body != nil && req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		clone.Body = body
	}

	return clone, nil
}

type retryableStatusError struct {
	code int
}

func (e retryableStatusError) Error() string {
	return fmt.Sprintf("retryable status: %d", e.code)
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

func normalizeRetryStatus(codes []int) map[int]struct{} {
	if len(codes) == 0 {
		codes = []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout}
	}

	set := make(map[int]struct{}, len(codes))
	for _, code := range codes {
		set[code] = struct{}{}
	}
	return set
}

func normalizeRetryMethods(methods []string) map[string]struct{} {
	if len(methods) == 0 {
		methods = []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions}
	}
	set := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		set[strings.ToUpper(method)] = struct{}{}
	}
	return set
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
