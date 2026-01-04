package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/wyfcoding/pkg/config"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	chOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "clickhouse_ops_total",
			Help: "The total number of clickhouse operations",
		},
		[]string{"database", "status"},
	)
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

// NewClient 创建一个新的原生 ClickHouse 客户端。
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

	return &MeasuredConn{Conn: conn, db: cfg.Database}, nil
}

// MeasuredConn 包装 driver.Conn 以添加指标。
type MeasuredConn struct {
	driver.Conn
	db string
}

func (c *MeasuredConn) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	start := time.Now()
	rows, err := c.Conn.Query(ctx, query, args...)
	c.recordMetrics(start, err)
	return rows, err
}

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

// BatchWriter 提供了批量写入 ClickHouse 的能力。

type BatchWriter struct {
	conn driver.Conn

	query string

	batchSize int

	timeout time.Duration
}

// NewBatchWriter 创建一个新的批量写入器。

func NewBatchWriter(conn driver.Conn, query string, batchSize int, timeout time.Duration) *BatchWriter {

	return &BatchWriter{

		conn: conn,

		query: query,

		batchSize: batchSize,

		timeout: timeout,
	}

}

// Write 批量写入数据。

func (w *BatchWriter) Write(ctx context.Context, data [][]any) error {

	if len(data) == 0 {

		return nil

	}

	batch, err := w.conn.PrepareBatch(ctx, w.query)

	if err != nil {

		return fmt.Errorf("failed to prepare batch: %w", err)

	}

	for _, row := range data {

		if err := batch.Append(row...); err != nil {

			return fmt.Errorf("failed to append row: %w", err)

		}

	}

	start := time.Now()

	err = batch.Send()

	// 记录指标

	duration := time.Since(start).Seconds()

	status := "success"

	if err != nil {

		status = "error"

	}

	chOps.WithLabelValues("batch", status).Inc()

	chDuration.WithLabelValues("batch").Observe(duration)

	return err

}
