// Package graph 提供 Neo4j 图数据库的统一接入与治理能力。
// 生成摘要:
// 1) 增加 Neo4j 客户端治理封装，包含限流、熔断、并发保护与慢查询监控。
// 2) 统一指标与链路追踪标签输出，便于跨服务观测。
// 假设:
// 1) 服务侧已传入有效的 Neo4j 连接配置与日志组件。
package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/tracing"
	"golang.org/x/time/rate"
)

var (
	// ErrRateLimit 表示触发图数据库限流。
	ErrRateLimit = errors.New("graph rate limit exceeded")
	// ErrConcurrencyLimit 表示触发图数据库并发限制。
	ErrConcurrencyLimit = errors.New("graph concurrency limit exceeded")
	// ErrGraphQuery 表示图数据库执行失败。
	ErrGraphQuery = errors.New("graph query failed")
)

const (
	defaultRateLimit = 1500
	defaultBurst     = 150
)

// Config 定义图数据库客户端的初始化参数。
type Config struct {
	config.Neo4jConfig
	BreakerConfig  config.CircuitBreakerConfig
	ServiceName    string
	Limiter        limiter.Limiter
	MaxConcurrency int
	SlowThreshold  time.Duration
}

// Client 封装 Neo4j 客户端，提供统一治理能力。
type Client struct {
	driver          neo4j.Driver
	dbName          string
	logger          *logging.Logger
	breaker         *breaker.Breaker
	limiter         limiter.Limiter
	concurrency     limiter.ConcurrencyLimiter
	slowThreshold   time.Duration
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	slowRequests    *prometheus.CounterVec
}

// NewClient 创建具备治理能力的图数据库客户端。
func NewClient(cfg Config, logger *logging.Logger, metricsInstance *metrics.Metrics) (*Client, error) {
	if metricsInstance == nil {
		metricsInstance = metrics.NewMetrics(cfg.ServiceName)
	}

	driver, err := neo4j.NewDriver(cfg.URI, neo4j.BasicAuth(cfg.Username, cfg.Password, ""))
	if err != nil {
		return nil, fmt.Errorf("failed to create neo4j driver: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, fmt.Errorf("failed to verify neo4j connectivity: %w", err)
	}

	if logger != nil {
		logger.Info("graph client initialized", "uri", cfg.URI)
	}

	limit := cfg.Limiter
	if limit == nil {
		limit = limiter.NewLocalLimiter(rate.Limit(defaultRateLimit), defaultBurst)
	}

	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "neo4j-" + cfg.ServiceName,
		Config: cfg.BreakerConfig,
	}, metricsInstance)

	requestsTotal := metricsInstance.NewCounterVec(&prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "neo4j",
		Name:      "requests_total",
		Help:      "Neo4j client request count",
	}, []string{"database", "op", "status"})

	requestDuration := metricsInstance.NewHistogramVec(&prometheus.HistogramOpts{
		Namespace: "pkg",
		Subsystem: "neo4j",
		Name:      "request_duration_seconds",
		Help:      "Neo4j client request latency",
		Buckets:   prometheus.DefBuckets,
	}, []string{"database", "op"})

	slowRequests := metricsInstance.NewCounterVec(&prometheus.CounterOpts{
		Namespace: "pkg",
		Subsystem: "neo4j",
		Name:      "slow_requests_total",
		Help:      "Neo4j slow request count",
	}, []string{"database", "op"})

	client := &Client{
		driver:          driver,
		dbName:          "neo4j",
		logger:          logger,
		breaker:         cb,
		limiter:         limit,
		concurrency:     limiter.NewSemaphoreLimiter(cfg.MaxConcurrency),
		slowThreshold:   cfg.SlowThreshold,
		requestsTotal:   requestsTotal,
		requestDuration: requestDuration,
		slowRequests:    slowRequests,
	}

	return client, nil
}

// Close 优雅关闭图数据库连接。
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.driver == nil {
		return nil
	}
	return c.driver.Close(ctx)
}

// SetDatabase 设置目标数据库名称。
func (c *Client) SetDatabase(name string) {
	if name != "" {
		c.dbName = name
	}
}

// ExecuteQuery 执行单条 Cypher 查询。
func (c *Client) ExecuteQuery(ctx context.Context, cypher string, params map[string]any) (*neo4j.EagerResult, error) {
	var result *neo4j.EagerResult
	if err := c.execute(ctx, "query", cypher, func(execCtx context.Context) error {
		res, err := neo4j.ExecuteQuery(execCtx, c.driver, cypher, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase(c.dbName))
		if err != nil {
			return err
		}
		result = res
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// ReadSession 执行只读会话逻辑。
func (c *Client) ReadSession(ctx context.Context, work func(neo4j.Session) error) error {
	return c.session(ctx, neo4j.AccessModeRead, work)
}

// Session 执行指定访问模式的会话逻辑（兼容通用调用模式）。
func (c *Client) Session(ctx context.Context, mode neo4j.AccessMode, work func(neo4j.Session) error) error {
	return c.session(ctx, mode, work)
}

// WriteSession 执行写入会话逻辑。
func (c *Client) WriteSession(ctx context.Context, work func(neo4j.Session) error) error {
	return c.session(ctx, neo4j.AccessModeWrite, work)
}

func (c *Client) session(ctx context.Context, mode neo4j.AccessMode, work func(neo4j.Session) error) error {
	op := "read"
	if mode == neo4j.AccessModeWrite {
		op = "write"
	}

	return c.execute(ctx, op, "", func(execCtx context.Context) error {
		session := c.driver.NewSession(execCtx, neo4j.SessionConfig{
			DatabaseName: c.dbName,
			AccessMode:   mode,
		})
		defer session.Close(execCtx)

		return work(session)
	})
}

func (c *Client) execute(ctx context.Context, op, statement string, fn func(context.Context) error) error {
	if c == nil {
		return ErrGraphQuery
	}

	if err := c.concurrency.Acquire(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrConcurrencyLimit, err)
	}
	defer c.concurrency.Release()

	allowed, err := c.limiter.Allow(ctx, "neo4j:"+op)
	if err != nil {
		c.logWarn(ctx, "neo4j limiter error", "error", err)
	}
	if !allowed {
		c.record(op, "rate_limited", 0)
		return ErrRateLimit
	}

	start := time.Now()

	_, execErr := c.breaker.Execute(func() (any, error) {
		execCtx, span := tracing.Tracer().Start(ctx, "Neo4j."+op)
		defer span.End()

		tracing.AddTag(execCtx, "db.system", "neo4j")
		tracing.AddTag(execCtx, "db.name", c.dbName)
		if statement != "" {
			tracing.AddTag(execCtx, "db.statement", statement)
		}

		err := fn(execCtx)
		if err != nil {
			tracing.SetError(execCtx, err)
		}
		return nil, err
	})

	duration := time.Since(start)
	status := "success"
	if execErr != nil {
		status = "error"
	}

	c.record(op, status, duration)
	c.checkSlow(ctx, op, statement, duration)

	if execErr != nil {
		return fmt.Errorf("%w: %v", ErrGraphQuery, execErr)
	}

	return nil
}

func (c *Client) record(op, status string, duration time.Duration) {
	if c.requestsTotal != nil {
		c.requestsTotal.WithLabelValues(c.dbName, op, status).Inc()
	}
	if c.requestDuration != nil && duration > 0 {
		c.requestDuration.WithLabelValues(c.dbName, op).Observe(duration.Seconds())
	}
}

func (c *Client) checkSlow(ctx context.Context, op, statement string, duration time.Duration) {
	if c.slowThreshold <= 0 || duration < c.slowThreshold {
		return
	}
	if c.slowRequests != nil {
		c.slowRequests.WithLabelValues(c.dbName, op).Inc()
	}

	c.logWarn(ctx, "neo4j slow query", "op", op, "duration", duration, "statement", statement)
}

func (c *Client) logWarn(ctx context.Context, msg string, args ...any) {
	if c.logger != nil {
		c.logger.WarnContext(ctx, msg, args...)
		return
	}
	logging.Warn(ctx, msg, args...)
}
