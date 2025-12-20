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

// NewClient creates a new native ClickHouse client.
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

func (c *MeasuredConn) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	start := time.Now()
	rows, err := c.Conn.Query(ctx, query, args...)
	c.recordMetrics(start, err)
	return rows, err
}

func (c *MeasuredConn) Exec(ctx context.Context, query string, args ...interface{}) error {
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
