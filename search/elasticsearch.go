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
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/tracing"
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
	es              *elasticsearch.Client
	logger          *logging.Logger
	cb              *breaker.Breaker
	limiter         limiter.Limiter
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	slowThreshold   time.Duration
}

// Config 定义了初始化搜索客户端所需的各项参数.
type Config struct {
	Username      string
	Password      string
	ServiceName   string
	Addresses     []string
	BreakerConfig config.CircuitBreakerConfig
	SlowThreshold time.Duration
	MaxRetries    int
}

// NewClient 创建具备全方位治理能力的 ES 客户端.
func NewClient(cfg *Config, logger *logging.Logger, metricsInstance *metrics.Metrics) (*Client, error) {
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

	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "Elasticsearch-" + cfg.ServiceName,
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

	l := limiter.NewLocalLimiter(esDefaultRate, esDefaultBurst)

	return &Client{
		es:              esClient,
		logger:          logger,
		slowThreshold:   cfg.SlowThreshold,
		cb:              cb,
		limiter:         l,
		requestsTotal:   reqTotal,
		requestDuration: reqDuration,
	}, nil
}

// Search 执行带全方位治理的搜索.
func (c *Client) Search(ctx context.Context, index string, query map[string]any, results any) error {
	allowed, errLimit := c.limiter.Allow(ctx, "es-search")
	if errLimit != nil {
		c.logger.ErrorContext(ctx, "limiter error", "error", errLimit)
	}

	if !allowed {
		return ErrRateLimit
	}

	operation := "search"
	start := time.Now()

	_, err := c.cb.Execute(func() (any, error) {
		searchCtx, span := tracing.StartSpan(ctx, "ES.Search")
		defer span.End()

		queryJSON, errMarshal := json.Marshal(query)
		if errMarshal != nil {
			return nil, fmt.Errorf("marshal query failed: %w", errMarshal)
		}

		tracing.AddTag(searchCtx, "db.system", "elasticsearch")
		tracing.AddTag(searchCtx, "db.index", index)
		tracing.AddTag(searchCtx, "db.statement", string(queryJSON))

		res, errSearch := c.es.Search(
			c.es.Search.WithContext(searchCtx),
			c.es.Search.WithIndex(index),
			c.es.Search.WithBody(bytes.NewReader(queryJSON)),
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
		c.checkSlow(searchCtx, index, queryJSON, time.Since(start))

		return struct{}{}, nil
	})

	return err
}

// Index 创建或在索引中更新指定的文档.
func (c *Client) Index(ctx context.Context, index, documentID string, document any) error {
	operation := "index"
	start := time.Now()

	_, err := c.cb.Execute(func() (any, error) {
		indexCtx, span := tracing.StartSpan(ctx, "ES.Index")
		defer span.End()

		data, errMarshal := json.Marshal(document)
		if errMarshal != nil {
			return nil, fmt.Errorf("failed to marshal document: %w", errMarshal)
		}

		res, errIdx := c.es.Index(
			index,
			bytes.NewReader(data),
			c.es.Index.WithContext(indexCtx),
			c.es.Index.WithDocumentID(documentID),
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
	})

	return err
}

// Delete 从索引中安全删除指定的文档.
func (c *Client) Delete(ctx context.Context, index, documentID string) error {
	operation := "delete"
	start := time.Now()

	_, err := c.cb.Execute(func() (any, error) {
		deleteCtx, span := tracing.StartSpan(ctx, "ES.Delete")
		defer span.End()

		res, errDel := c.es.Delete(
			index,
			documentID,
			c.es.Delete.WithContext(deleteCtx),
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
	})

	return err
}

// Bulk 批量操作接口.
func (c *Client) Bulk(ctx context.Context, body io.Reader) error {
	res, err := c.es.Bulk(body, c.es.Bulk.WithContext(ctx))
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

func (c *Client) checkSlow(ctx context.Context, index string, query []byte, cost time.Duration) {
	if c.slowThreshold > 0 && cost > c.slowThreshold {
		c.logger.WarnContext(ctx, "es slow query",
			"index", index,
			"cost", cost.String(),
			"q", string(query))
	}
}
