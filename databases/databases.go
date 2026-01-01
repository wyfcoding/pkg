package databases

import (
	"context"
	"fmt"
	"time"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"gorm.io/plugin/opentelemetry/tracing"
)

// DB 包装了 GORM 实例及其治理组件
type DB struct {
	*gorm.DB
	breaker *breaker.Breaker
	metrics *metrics.Metrics
	name    string
}

// RawDB 返回底层的 gorm.DB 实例
func (db *DB) RawDB() *gorm.DB {
	return db.DB
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// NewDB 创建具备全方位治理能力的 GORM 实例
func NewDB(cfg config.DatabaseConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) (*DB, error) {
	gormLogger := logging.NewGormLogger(logger, cfg.SlowThreshold)

	gormConfig := &gorm.Config{
		Logger: gormLogger,
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	}

	var dialector gorm.Dialector
	switch cfg.Driver {
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	case "clickhouse":
		dialector = clickhouse.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	rawDB, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, err
	}

	// 1. 注入 OpenTelemetry Tracing
	if err := rawDB.Use(tracing.NewPlugin()); err != nil {
		return nil, err
	}

	sqlDB, err := rawDB.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}

	// 2. 初始化项目标准熔断器
	dbBreaker := breaker.NewBreaker(breaker.Settings{
		Name:   "DB-" + cfg.Driver,
		Config: cbCfg,
	}, m)

	// 3. 注册连接池指标到统一 Registry
	poolMax := m.NewGaugeVec(prometheus.GaugeOpts{Name: "db_pool_max_open", Help: "Max open connections"}, []string{"db"})
	poolOpen := m.NewGaugeVec(prometheus.GaugeOpts{Name: "db_pool_open", Help: "Current open connections"}, []string{"db"})
	poolInUse := m.NewGaugeVec(prometheus.GaugeOpts{Name: "db_pool_in_use", Help: "Current in-use connections"}, []string{"db"})

	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			stats := sqlDB.Stats()
			poolMax.WithLabelValues(cfg.Driver).Set(float64(stats.MaxOpenConnections))
			poolOpen.WithLabelValues(cfg.Driver).Set(float64(stats.OpenConnections))
			poolInUse.WithLabelValues(cfg.Driver).Set(float64(stats.InUse))
		}
	}()

	return &DB{
		DB:      rawDB,
		breaker: dbBreaker,
		metrics: m,
		name:    cfg.Driver,
	}, nil
}

// ExecSafe 执行带熔断保护的数据库操作
func (db *DB) ExecSafe(fn func(tx *gorm.DB) error) error {
	_, err := db.breaker.Execute(func() (any, error) {
		return nil, fn(db.DB)
	})
	return err
}

// WithTransaction 事务助手：全自动处理，并注入熔断保护
func (db *DB) WithTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	_, err := db.breaker.Execute(func() (any, error) {
		return nil, db.DB.WithContext(ctx).Transaction(fn)
	})
	return err
}
