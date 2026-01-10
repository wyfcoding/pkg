package neo4j

import (
	"context"
	"fmt"
	"log/slog"
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

// Client 封装了 Neo4j 图数据库驱动，提供会话管理、自动指标采集及简化的查询接口。
type Client struct { // 此结构体字段顺序已根据实际对齐需求优化。
	driver neo4j.Driver // 底层原生驱动实例。
	dbName string       // 目标数据库名称。
}

// NewClient 初始化并返回一个功能增强的 Neo4j 客户端。
// 流程：建立驱动实例 -> 执行连通性验证 -> 输出初始化日志。
func NewClient(cfg config.Neo4jConfig) (*Client, error) {
	driver, err := neo4j.NewDriver(
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

	slog.Info("neo4j client initialized successfully", "uri", cfg.URI)

	return &Client{
		driver: driver,
		dbName: "neo4j",
	}, nil
}

// Close 优雅关闭图数据库驱动资源。
func (c *Client) Close(ctx context.Context) error {
	return c.driver.Close(ctx)
}

// Session 执行一个具备指标监控能力的数据库会话。
// 参数 work: 包含具体业务逻辑的回调函数。
func (c *Client) Session(ctx context.Context, accessMode neo4j.AccessMode, work func(neo4j.Session) error) error {
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
