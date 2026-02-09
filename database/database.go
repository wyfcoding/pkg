package database

import (
	"errors"
	"sync"
	"time"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/xerrors"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

var (
	defaultDB *DB
	// ErrTransactionFailed 事务执行失败.
	ErrTransactionFailed = errors.New("transaction failed")
)

const (
	defaultSlowThreshold = 200 * time.Millisecond
	errBadRequest        = 400
)

// DB 封装了 GORM 实例.
type DB struct {
	mu sync.RWMutex
	*gorm.DB
	cfg     *config.DatabaseConfig
	breaker *breaker.Breaker
	logger  *logging.Logger
}

// Default 返回全局默认的数据库连接实例.
func Default() *DB {
	return defaultDB
}

// SetDefault 设置全局默认数据库连接.
func SetDefault(db *DB) {
	defaultDB = db
}

// NewDB 初始化并返回一个功能增强的数据库连接封装.
func NewDB(cfg config.DatabaseConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) (*DB, error) {
	if logger == nil {
		logger = logging.Default()
	}
	var dialer gorm.Dialector

	dsn := cfg.DSN
	switch cfg.Driver {
	case "mysql":
		dialer = mysql.Open(dsn)
	case "postgres":
		dialer = postgres.Open(dsn)
	case "clickhouse":
		dialer = clickhouse.Open(dsn)
	default:
		return nil, xerrors.New(xerrors.ErrInvalidArg, errBadRequest, "unsupported database driver", cfg.Driver, nil)
	}

	slowThreshold := cfg.SlowThreshold
	if slowThreshold <= 0 {
		slowThreshold = defaultSlowThreshold
	}
	gormLogger := logging.NewGormLogger(logger, slowThreshold).LogMode(cfg.LogLevel)

	gormDB, err := gorm.Open(dialer, &gorm.Config{
		Logger:      gormLogger,
		PrepareStmt: true,
	})
	if err != nil {
		return nil, xerrors.WrapInternal(err, "failed to open database connection")
	}

	if errTracing := gormDB.Use(tracing.NewPlugin()); errTracing != nil {
		return nil, xerrors.WrapInternal(errTracing, "failed to register gorm otel plugin")
	}

	sqlDB, errDB := gormDB.DB()
	if errDB != nil {
		return nil, xerrors.WrapInternal(errDB, "failed to get underlying sql.DB")
	}

	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	cb := breaker.NewBreaker(breaker.Settings{
		Name:         "database-" + cfg.Driver,
		Config:       cbCfg,
		FailureRatio: 0,
		MinRequests:  0,
	}, m)

	db := &DB{
		DB:      gormDB,
		cfg:     &cfg,
		breaker: cb,
		logger:  logger,
	}

	// 注册连接池指标
	db.registerMetrics(m)

	return db, nil
}

func (db *DB) registerMetrics(m *metrics.Metrics) {
	sqlDB, err := db.DB.DB()
	if err != nil {
		return
	}

	m.NewGaugeFunc(&prometheus.GaugeOpts{
		Name:        "db_max_open_connections",
		Help:        "Maximum number of open connections to the database.",
		ConstLabels: prometheus.Labels{"driver": db.cfg.Driver},
	}, func() float64 { return float64(sqlDB.Stats().MaxOpenConnections) })

	m.NewGaugeFunc(&prometheus.GaugeOpts{
		Name:        "db_open_connections",
		Help:        "The number of established connections both in use and idle.",
		ConstLabels: prometheus.Labels{"driver": db.cfg.Driver},
	}, func() float64 { return float64(sqlDB.Stats().OpenConnections) })

	m.NewGaugeFunc(&prometheus.GaugeOpts{
		Name:        "db_in_use_connections",
		Help:        "The number of connections currently in use.",
		ConstLabels: prometheus.Labels{"driver": db.cfg.Driver},
	}, func() float64 { return float64(sqlDB.Stats().InUse) })

	m.NewGaugeFunc(&prometheus.GaugeOpts{
		Name:        "db_idle_connections",
		Help:        "The number of idle connections.",
		ConstLabels: prometheus.Labels{"driver": db.cfg.Driver},
	}, func() float64 { return float64(sqlDB.Stats().Idle) })
}

// Ping 检查数据库连接是否存活.
func (db *DB) Ping() error {
	orm, _, _ := db.snapshot()
	if orm == nil {
		return errors.New("database is nil")
	}
	sqlDB, err := orm.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// Transaction 封装了带熔断保护的事务逻辑.
func (db *DB) Transaction(fc func(tx *gorm.DB) error) error {
	orm, cb, _ := db.snapshot()
	if orm == nil {
		return errors.New("database is nil")
	}
	exec := func() (any, error) {
		errTx := orm.Transaction(fc)
		if errTx != nil {
			return nil, xerrors.Wrap(errTx, xerrors.ErrInternal, ErrTransactionFailed.Error())
		}

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

// RawDB 暴露原始 GORM 实例.
func (db *DB) RawDB() *gorm.DB {
	orm, _, _ := db.snapshot()
	return orm
}

// UpdateConfig 使用最新配置刷新数据库连接。
func (db *DB) UpdateConfig(cfg config.DatabaseConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) error {
	if db == nil {
		return errors.New("database is nil")
	}
	newDB, err := NewDB(cfg, cbCfg, logger, m)
	if err != nil {
		return err
	}

	db.mu.Lock()
	oldDB := db.DB
	db.DB = newDB.DB
	db.cfg = newDB.cfg
	db.breaker = newDB.breaker
	db.logger = newDB.logger
	db.mu.Unlock()

	if oldDB != nil {
		if sqlDB, err := oldDB.DB(); err == nil {
			if errClose := sqlDB.Close(); errClose != nil && db.logger != nil {
				db.logger.Error("failed to close old database", "error", errClose)
			}
		}
	}

	if db.logger != nil {
		db.logger.Info("database updated", "driver", cfg.Driver)
	}

	return nil
}

// RegisterReloadHook 注册数据库热更新回调。
func RegisterReloadHook(db *DB, logger *logging.Logger, m *metrics.Metrics) {
	if db == nil {
		return
	}
	config.RegisterReloadHook(func(updated *config.Config) {
		if updated == nil {
			return
		}
		if err := db.UpdateConfig(updated.Data.Database, updated.CircuitBreaker, logger, m); err != nil {
			if logger == nil {
				logger = logging.Default()
			}
			logger.Error("database reload failed", "error", err)
		}
	})
}

func (db *DB) snapshot() (*gorm.DB, *breaker.Breaker, *logging.Logger) {
	if db == nil {
		return nil, nil, logging.Default()
	}
	db.mu.RLock()
	orm := db.DB
	cb := db.breaker
	logger := db.logger
	db.mu.RUnlock()
	if logger == nil {
		logger = logging.Default()
	}
	return orm, cb, logger
}
