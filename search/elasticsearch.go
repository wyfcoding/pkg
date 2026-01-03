package search

import (
	"bytes"
	"context"
	"encoding/json"
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

// Client 封装了顶级治理能力的 ES 客户端
type Client struct {
	es            *elasticsearch.Client
	logger        *logging.Logger
	slowThreshold time.Duration
	cb            *breaker.Breaker
	limiter       limiter.Limiter

	// 指标组件
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
}

// Config 搜索配置
type Config struct {
	Addresses     []string
	Username      string
	Password      string
	SlowThreshold time.Duration
	MaxRetries    int
	ServiceName   string
	BreakerConfig config.CircuitBreakerConfig // 显式传入熔断配置
}

// NewClient 创建具备全方位治理能力的 ES 客户端
func NewClient(cfg Config, logger *logging.Logger, m *metrics.Metrics) (*Client, error) {
	// 1. 深度优化 HTTP Transport
	tp := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	esCfg := elasticsearch.Config{
		Addresses:     cfg.Addresses,
		Username:      cfg.Username,
		Password:      cfg.Password,
		Transport:     tp,
		RetryOnStatus: []int{502, 503, 504, 429},
		MaxRetries:    cfg.MaxRetries,
	}

	esClient, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		return nil, err
	}

	// 2. 初始化项目标准的熔断器
	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "Elasticsearch-" + cfg.ServiceName,
		Config: cfg.BreakerConfig,
	}, m)

	// 3. 指标初始化
	reqTotal := m.NewCounterVec(prometheus.CounterOpts{
		Name: "es_client_requests_total",
		Help: "Elasticsearch client request count",
	}, []string{"index", "op", "status"})

	reqDuration := m.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "es_client_request_duration_seconds",
		Help:    "Elasticsearch client request latency",
		Buckets: prometheus.DefBuckets,
	}, []string{"index", "op"})

	// 4. 初始化本地限流器
	l := limiter.NewLocalLimiter(2000, 200)

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

// Search 执行带全方位治理的搜索
func (c *Client) Search(ctx context.Context, index string, query map[string]any, results any) error {
	allowed, err := c.limiter.Allow(ctx, "es-search")
	if err != nil {
		c.logger.ErrorContext(ctx, "limiter error", "error", err)
	}
	if !allowed {
		return fmt.Errorf("es rate limit exceeded")
	}

	operation := "search"
	start := time.Now()

	// 熔断执行 (使用 pkg/breaker)
	_, err = c.cb.Execute(func() (any, error) {
		ctx, span := tracing.StartSpan(ctx, "ES.Search")
		defer span.End()

		queryJSON, err := json.Marshal(query)
		if err != nil {
			return nil, fmt.Errorf("marshal query failed: %w", err)
		}
		tracing.AddTag(ctx, "db.system", "elasticsearch")
		tracing.AddTag(ctx, "db.index", index)
		tracing.AddTag(ctx, "db.statement", string(queryJSON))

		res, err := c.es.Search(
			c.es.Search.WithContext(ctx),
			c.es.Search.WithIndex(index),
			c.es.Search.WithBody(bytes.NewReader(queryJSON)),
		)
		if err != nil {
			c.record(index, operation, "error", start)
			tracing.SetError(ctx, err)
			return nil, err
		}
		defer func() {
			if cerr := res.Body.Close(); cerr != nil {
				c.logger.ErrorContext(ctx, "failed to close ES response body", "error", cerr)
			}
		}()

		if res.IsError() {
			c.record(index, operation, "fail", start)
			return nil, fmt.Errorf("es search error: %s", res.Status())
		}

		if err := json.NewDecoder(res.Body).Decode(results); err != nil {
			return nil, err
		}

		c.record(index, operation, "success", start)
		c.checkSlow(ctx, index, queryJSON, time.Since(start))
		return nil, nil
	})

	return err
}

// Index 创建或更新文档
func (c *Client) Index(ctx context.Context, index string, documentID string, document any) error {
	operation := "index"
	start := time.Now()

	_, err := c.cb.Execute(func() (any, error) {
		ctx, span := tracing.StartSpan(ctx, "ES.Index")
		defer span.End()

		data, err := json.Marshal(document)
		if err != nil {
			return nil, err
		}

		res, err := c.es.Index(
			index,
			bytes.NewReader(data),
			c.es.Index.WithContext(ctx),
			c.es.Index.WithDocumentID(documentID),
		)
		if err != nil {
			c.record(index, operation, "error", start)
			tracing.SetError(ctx, err)
			return nil, err
		}
		defer res.Body.Close()

		if res.IsError() {
			c.record(index, operation, "fail", start)
			return nil, fmt.Errorf("es index error: %s", res.Status())
		}

		c.record(index, operation, "success", start)
		return nil, nil
	})

	return err
}

// Delete 删除文档
func (c *Client) Delete(ctx context.Context, index string, documentID string) error {
	operation := "delete"
	start := time.Now()

	_, err := c.cb.Execute(func() (any, error) {
		ctx, span := tracing.StartSpan(ctx, "ES.Delete")
		defer span.End()

		res, err := c.es.Delete(
			index,
			documentID,
			c.es.Delete.WithContext(ctx),
		)
		if err != nil {
			c.record(index, operation, "error", start)
			tracing.SetError(ctx, err)
			return nil, err
		}
		defer res.Body.Close()

		if res.IsError() && res.StatusCode != 404 {
			c.record(index, operation, "fail", start)
			return nil, fmt.Errorf("es delete error: %s", res.Status())
		}

		c.record(index, operation, "success", start)
		return nil, nil
	})

	return err
}

// Bulk 批量操作接口
func (c *Client) Bulk(ctx context.Context, body io.Reader) error {
	res, err := c.es.Bulk(body, c.es.Bulk.WithContext(ctx))
	if err != nil {
		return err
	}
	defer func() {
		if cerr := res.Body.Close(); cerr != nil {
			c.logger.ErrorContext(ctx, "failed to close ES bulk response body", "error", cerr)
		}
	}()
	if res.IsError() {
		return fmt.Errorf("es bulk error: %s", res.Status())
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
