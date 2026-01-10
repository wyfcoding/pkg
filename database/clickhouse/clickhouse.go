// Package clickhouse 提供了 ClickHouse 分析型数据库的客户端封装，支持连接池管理、Prometheus 指标监控及高效的批量写入能力。
package clickhouse

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/wyfcoding/pkg/config"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// chOps 记录 ClickHouse 各类操作的次数。
	chOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clickhouse_ops_total",
			Help: "The total number of clickhouse operations",
		},
		[]string{"database", "status"},
	)
	// chDuration 记录 ClickHouse 操作的延迟分布。
	chDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "clickhouse_duration_seconds",
			Help:    "The duration of clickhouse operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"database"},
	)
)

func init() {
	prometheus.MustRegister(chOps, chDuration)
}

// NewClient 初始化并返回一个带有性能指标监控的 ClickHouse 驱动连接。
// 流程：建立连接 -> 执行 Ping 校验 -> 包装监控层。
func NewClient(cfg config.ClickHouseConfig) (driver.Conn, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		Compression:     &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open clickhouse connection: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	slog.Info("clickhouse client initialized successfully", "addr", cfg.Addr, "db", cfg.Database)
	return &MeasuredConn{Conn: conn, db: cfg.Database}, nil
}

// MeasuredConn 通过装饰器模式扩展原生驱动，自动采集 Prometheus 指标。
type MeasuredConn struct { //nolint:govet
	driver.Conn
	db string
}

// Query 执行 SQL 查询并记录耗时。
func (c *MeasuredConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	start := time.Now()
	rows, err := c.Conn.Query(ctx, query, args...)
	c.recordMetrics(start, err)
	return rows, err
}

// Exec 执行非查询 SQL 并记录耗时。
func (c *MeasuredConn) Exec(ctx context.Context, query string, args ...any) error {
	start := time.Now()
	err := c.Conn.Exec(ctx, query, args...)
	c.recordMetrics(start, err)
	return err
}

func (c *MeasuredConn) recordMetrics(start time.Time, err error) {
	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
	}
	chOps.WithLabelValues(c.db, status).Inc()
	chDuration.WithLabelValues(c.db).Observe(duration)
}

// BatchWriter 封装了 ClickHouse 官方的高性能 Batch 接口，提供更易用的批量写入能力。
type BatchWriter struct { //nolint:govet
	conn      driver.Conn
	query     string
	batchSize int
	timeout   time.Duration
}

// NewBatchWriter 构造一个新的批量写入执行器。
func NewBatchWriter(conn driver.Conn, query string, batchSize int, timeout time.Duration) *BatchWriter {
	return &BatchWriter{
		conn:      conn,
		query:     query,
		batchSize: batchSize,
		timeout:   timeout,
	}
}

// Write 执行批次写入操作。
func (w *BatchWriter) Write(ctx context.Context, data [][]any) error {
	if len(data) == 0 {
		return nil
	}

	batch, err := w.conn.PrepareBatch(ctx, w.query)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, row := range data {
		if errAppend := batch.Append(row...); errAppend != nil {
			return fmt.Errorf("failed to append row: %w", errAppend)
		}
	}

	start := time.Now()
	err = batch.Send()

	// 记录批次执行指标。
	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
	}
	chOps.WithLabelValues("batch_write", status).Inc()
	chDuration.WithLabelValues("batch_write").Observe(duration)

	return err
}
