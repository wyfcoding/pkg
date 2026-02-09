package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/tracing"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// ErrRateLimit 限流错误.
	ErrRateLimit = errors.New("es rate limit exceeded")
	// ErrSearchES 搜索执行错误.
	ErrSearchES = errors.New("es search error")
	// ErrIndexES 索引执行错误.
	ErrIndexES = errors.New("es index error")
	// ErrDeleteES 删除执行错误.
	ErrDeleteES = errors.New("es delete error")
	// ErrBulkES 批量执行错误.
	ErrBulkES = errors.New("es bulk error")
)

const (
	esDialTimeout     = 30 * time.Second
	esKeepAlive       = 30 * time.Second
	esMaxIdleConns    = 100
	esIdleConnTimeout = 90 * time.Second
	esTLSHandshake    = 10 * time.Second
	esExpectContinue  = 1 * time.Second
	esDefaultRate     = 2000
	esDefaultBurst    = 200
)

// Client 封装了具备高级治理能力的 Elasticsearch 客户端.
type Client struct {
	mu              sync.RWMutex
	es              *elasticsearch.Client
	logger          *logging.Logger
	metrics         *metrics.Metrics
	cb              *breaker.Breaker
	limiter         limiter.Limiter
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	slowThreshold   time.Duration
}

// Config 定义了初始化搜索客户端所需的各项参数.
type Config struct {
	Limiter     limiter.Limiter // 可选：注入自定义限流器（如分布式限流器）。
	ServiceName string
	config.ElasticsearchConfig
	BreakerConfig config.CircuitBreakerConfig
	SlowThreshold time.Duration
	MaxRetries    int
}

// NewClient 创建具备全方位治理能力的 ES 客户端.
func NewClient(cfg *Config, logger *logging.Logger, metricsInstance *metrics.Metrics) (*Client, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "search"
	}
	if logger == nil {
		logger = logging.Default()
	}
	if metricsInstance == nil {
		metricsInstance = metrics.NewMetrics(serviceName)
	}

	esClient, err := newESClient(cfg)
	if err != nil {
		return nil, err
	}

	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "Elasticsearch-" + serviceName,
		Config: cfg.BreakerConfig,
	}, metricsInstance)

	reqTotal := metricsInstance.NewCounterVec(&prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "es",
		Name:      "requests_total",
		Help:      "Elasticsearch client request count",
	}, []string{"index", "op", "status"})

	reqDuration := metricsInstance.NewHistogramVec(&prometheus.HistogramOpts{
		Namespace: "pkg",
		Subsystem: "es",
		Name:      "request_duration_seconds",
		Help:      "Elasticsearch client request latency",
		Buckets:   prometheus.DefBuckets,
	}, []string{"index", "op"})

	l := cfg.Limiter
	if l == nil {
		l = limiter.NewLocalLimiter(esDefaultRate, esDefaultBurst)
	}

	return &Client{
		es:              esClient,
		logger:          logger,
		metrics:         metricsInstance,
		slowThreshold:   cfg.SlowThreshold,
		cb:              cb,
		limiter:         l,
		requestsTotal:   reqTotal,
		requestDuration: reqDuration,
	}, nil
}

// UpdateConfig 使用最新配置刷新 ES 客户端。
func (c *Client) UpdateConfig(cfg *Config) error {
	if c == nil {
		return errors.New("es client is nil")
	}
	if cfg == nil {
		return errors.New("config is nil")
	}
	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "search"
	}

	logger := c.logger
	if logger == nil {
		logger = logging.Default()
	}

	esClient, err := newESClient(cfg)
	if err != nil {
		return err
	}

	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "Elasticsearch-" + serviceName,
		Config: cfg.BreakerConfig,
	}, c.metrics)

	l := cfg.Limiter
	if l == nil {
		l = limiter.NewLocalLimiter(esDefaultRate, esDefaultBurst)
	}

	c.mu.Lock()
	c.es = esClient
	c.cb = cb
	c.limiter = l
	c.slowThreshold = cfg.SlowThreshold
	c.logger = logger
	c.mu.Unlock()

	logger.Info("es client updated", "addresses", cfg.Addresses)

	return nil
}

// RegisterReloadHook 注册 ES 客户端热更新回调。
// baseConfig 用于保留服务名、限流器等非配置中心字段。
func RegisterReloadHook(client *Client, baseConfig Config) {
	if client == nil {
		return
	}
	config.RegisterReloadHook(func(updated *config.Config) {
		if updated == nil {
			return
		}
		next := baseConfig
		next.ElasticsearchConfig = updated.Data.Elasticsearch
		next.BreakerConfig = updated.CircuitBreaker
		if err := client.UpdateConfig(&next); err != nil {
			logger := client.logger
			if logger == nil {
				logger = logging.Default()
			}
			logger.Error("es client reload failed", "error", err)
		}
	})
}

func newESClient(cfg *Config) (*elasticsearch.Client, error) {
	tp := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   esDialTimeout,
			KeepAlive: esKeepAlive,
		}).DialContext,
		MaxIdleConns:          esMaxIdleConns,
		IdleConnTimeout:       esIdleConnTimeout,
		TLSHandshakeTimeout:   esTLSHandshake,
		ExpectContinueTimeout: esExpectContinue,
		ForceAttemptHTTP2:     true,
	}

	esCfg := elasticsearch.Config{
		Addresses:     cfg.Addresses,
		Username:      cfg.Username,
		Password:      cfg.Password,
		Transport:     tp,
		MaxRetries:    cfg.MaxRetries,
		RetryOnStatus: []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusTooManyRequests},
	}

	esClient, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create es client: %w", err)
	}
	return esClient, nil
}

// Search 执行带全方位治理的搜索.
func (c *Client) Search(ctx context.Context, index string, query map[string]any, results any) error {
	esClient, logger, cb, limit, slowThreshold := c.snapshot()
	if esClient == nil {
		return errors.New("es client not initialized")
	}

	allowed := true
	if limit != nil {
		errLimit := error(nil)
		allowed, errLimit = limit.Allow(ctx, "es-search")
		if errLimit != nil {
			logger.ErrorContext(ctx, "limiter error", "error", errLimit)
		}
	}
	if !allowed {
		return ErrRateLimit
	}

	operation := "search"
	start := time.Now()

	exec := func() (any, error) {
		searchCtx, span := tracing.Tracer().Start(ctx, "ES.Search")
		defer span.End()

		queryJSON, errMarshal := json.Marshal(query)
		if errMarshal != nil {
			return nil, fmt.Errorf("marshal query failed: %w", errMarshal)
		}

		tracing.AddTag(searchCtx, "db.system", "elasticsearch")
		tracing.AddTag(searchCtx, "db.index", index)
		tracing.AddTag(searchCtx, "db.statement", string(queryJSON))

		res, errSearch := esClient.Search(
			esClient.Search.WithContext(searchCtx),
			esClient.Search.WithIndex(index),
			esClient.Search.WithBody(bytes.NewReader(queryJSON)),
		)
		if errSearch != nil {
			c.record(index, operation, "error", start)
			tracing.SetError(searchCtx, errSearch)

			return nil, fmt.Errorf("es search request failed: %w", errSearch)
		}
		defer res.Body.Close()

		if res.IsError() {
			c.record(index, operation, "fail", start)

			return nil, fmt.Errorf("%w: %s", ErrSearchES, res.Status())
		}

		if errDecode := json.NewDecoder(res.Body).Decode(results); errDecode != nil {
			return nil, fmt.Errorf("failed to decode es response: %w", errDecode)
		}

		c.record(index, operation, "success", start)
		c.checkSlow(searchCtx, logger, slowThreshold, index, queryJSON, time.Since(start))

		return struct{}{}, nil
	}

	var err error
	if cb != nil {
		_, err = cb.Execute(exec)
	} else {
		_, err = exec()
	}

	return err
}

// Index 创建或在索引中更新指定的文档.
func (c *Client) Index(ctx context.Context, index, documentID string, document any) error {
	esClient, _, cb, _, _ := c.snapshot()
	if esClient == nil {
		return errors.New("es client not initialized")
	}

	operation := "index"
	start := time.Now()

	exec := func() (any, error) {
		indexCtx, span := tracing.Tracer().Start(ctx, "ES.Index")
		defer span.End()

		data, errMarshal := json.Marshal(document)
		if errMarshal != nil {
			return nil, fmt.Errorf("failed to marshal document: %w", errMarshal)
		}

		res, errIdx := esClient.Index(
			index,
			bytes.NewReader(data),
			esClient.Index.WithContext(indexCtx),
			esClient.Index.WithDocumentID(documentID),
		)
		if errIdx != nil {
			c.record(index, operation, "error", start)
			tracing.SetError(indexCtx, errIdx)

			return nil, fmt.Errorf("es index request failed: %w", errIdx)
		}
		defer res.Body.Close()

		if res.IsError() {
			c.record(index, operation, "fail", start)

			return nil, fmt.Errorf("%w: %s", ErrIndexES, res.Status())
		}

		c.record(index, operation, "success", start)

		return struct{}{}, nil
	}

	var err error
	if cb != nil {
		_, err = cb.Execute(exec)
	} else {
		_, err = exec()
	}

	return err
}

// Delete 从索引中安全删除指定的文档.
func (c *Client) Delete(ctx context.Context, index, documentID string) error {
	esClient, _, cb, _, _ := c.snapshot()
	if esClient == nil {
		return errors.New("es client not initialized")
	}

	operation := "delete"
	start := time.Now()

	exec := func() (any, error) {
		deleteCtx, span := tracing.Tracer().Start(ctx, "ES.Delete")
		defer span.End()

		res, errDel := esClient.Delete(
			index,
			documentID,
			esClient.Delete.WithContext(deleteCtx),
		)
		if errDel != nil {
			c.record(index, operation, "error", start)
			tracing.SetError(deleteCtx, errDel)

			return nil, fmt.Errorf("es delete request failed: %w", errDel)
		}
		defer res.Body.Close()

		if res.IsError() && res.StatusCode != http.StatusNotFound {
			c.record(index, operation, "fail", start)

			return nil, fmt.Errorf("%w: %s", ErrDeleteES, res.Status())
		}

		c.record(index, operation, "success", start)

		return struct{}{}, nil
	}

	var err error
	if cb != nil {
		_, err = cb.Execute(exec)
	} else {
		_, err = exec()
	}

	return err
}

// Bulk 批量操作接口.
func (c *Client) Bulk(ctx context.Context, body io.Reader) error {
	esClient, _, _, _, _ := c.snapshot()
	if esClient == nil {
		return errors.New("es client not initialized")
	}
	res, err := esClient.Bulk(body, esClient.Bulk.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("bulk execution failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("%w: %s", ErrBulkES, res.Status())
	}

	return nil
}

func (c *Client) record(index, op, status string, start time.Time) {
	c.requestsTotal.WithLabelValues(index, op, status).Inc()
	c.requestDuration.WithLabelValues(index, op).Observe(time.Since(start).Seconds())
}

func (c *Client) checkSlow(ctx context.Context, logger *logging.Logger, slowThreshold time.Duration, index string, query []byte, cost time.Duration) {
	if slowThreshold > 0 && cost > slowThreshold {
		logger.WarnContext(ctx, "es slow query",
			"index", index,
			"cost", cost.String(),
			"q", string(query))
	}
}

func (c *Client) snapshot() (*elasticsearch.Client, *logging.Logger, *breaker.Breaker, limiter.Limiter, time.Duration) {
	if c == nil {
		return nil, logging.Default(), nil, nil, 0
	}
	c.mu.RLock()
	esClient := c.es
	logger := c.logger
	cb := c.cb
	limit := c.limiter
	slow := c.slowThreshold
	c.mu.RUnlock()

	if logger == nil {
		logger = logging.Default()
	}

	return esClient, logger, cb, limit, slow
}
