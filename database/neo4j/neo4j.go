package neo4j

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/wyfcoding/pkg/config"
)

var (
	neo4jOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "neo4j_ops_total",
			Help: "Total number of Neo4j operations",
		},
		[]string{"database", "op_type", "status"},
	)
	neo4jDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "neo4j_duration_seconds",
			Help:    "Duration of Neo4j operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"database", "op_type"},
	)
)

func init() {
	prometheus.MustRegister(neo4jOps, neo4jDuration)
}

// Client 是 Neo4j 驱动的封装。
type Client struct {
	driver neo4j.DriverWithContext
	dbName string
}

// NewClient 创建一个新的 Neo4j 客户端。
func NewClient(cfg config.Neo4jConfig) (*Client, error) {
	driver, err := neo4j.NewDriverWithContext(
		cfg.URI,
		neo4j.BasicAuth(cfg.Username, cfg.Password, ""),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create neo4j driver: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := driver.VerifyConnectivity(ctx); err != nil {
		driver.Close(ctx)
		return nil, fmt.Errorf("failed to verify neo4j connectivity: %w", err)
	}

	return &Client{
		driver: driver,
		dbName: "neo4j", // Default database name for Neo4j
	}, nil
}

// Close 关闭驱动程序。
func (c *Client) Close(ctx context.Context) error {
	return c.driver.Close(ctx)
}

// Session 执行具有指标监控的 Neo4j 会话操作。
func (c *Client) Session(ctx context.Context, accessMode neo4j.AccessMode, work func(neo4j.SessionWithContext) error) error {
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: c.dbName,
		AccessMode:   accessMode,
	})
	defer session.Close(ctx)

	opType := "read"
	if accessMode == neo4j.AccessModeWrite {
		opType = "write"
	}

	start := time.Now()
	err := work(session)

	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
	}

	neo4jOps.WithLabelValues(c.dbName, opType, status).Inc()
	neo4jDuration.WithLabelValues(c.dbName, opType).Observe(duration)

	return err
}

// ExecuteQuery 简化的查询执行接口。
func (c *Client) ExecuteQuery(ctx context.Context, cypher string, params map[string]any) (*neo4j.EagerResult, error) {
	start := time.Now()
	res, err := neo4j.ExecuteQuery(ctx, c.driver, cypher, params, neo4j.EagerResultTransformer, neo4j.ExecuteQueryWithDatabase(c.dbName))

	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
	}

	neo4jOps.WithLabelValues(c.dbName, "query", status).Inc()
	neo4jDuration.WithLabelValues(c.dbName, "query").Observe(duration)

	return res, err
}
