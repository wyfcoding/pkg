package database

import (
	"sync"
	"time"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/xerrors"

	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

var (
	defaultDB *DB
	dbOnce    sync.Once
)

// DB 封装了 GORM 实例，增加了单例管理和熔断/指标支持
type DB struct {
	*gorm.DB
	cfg     *config.DatabaseConfig
	breaker *breaker.Breaker
	logger  *logging.Logger
}

// Default 返回全局默认数据库连接
func Default() *DB {
	return defaultDB
}

// SetDefault 设置全局默认数据库连接
func SetDefault(db *DB) {
	defaultDB = db
}

// NewDB 初始化并返回一个新的数据库连接封装
func NewDB(cfg config.DatabaseConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) (*DB, error) {
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
		return nil, xerrors.New(xerrors.ErrInvalidArg, 400, "unsupported database driver", cfg.Driver, nil)
	}

	// 初始化 GORM，使用统一的日志封装
	gormDB, err := gorm.Open(dialer, &gorm.Config{
		Logger: logging.NewGormLogger(logger, 200*time.Millisecond),
		PrepareStmt: true,
	})
	if err != nil {
		return nil, xerrors.WrapInternal(err, "failed to open database connection")
	}

	// 注入 OpenTelemetry 插件实现自动链路追踪
	if err := gormDB.Use(tracing.NewPlugin()); err != nil {
		return nil, xerrors.WrapInternal(err, "failed to register gorm otel plugin")
	}

	// 获取底层 SQL DB 以配置连接池
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, xerrors.WrapInternal(err, "failed to get underlying sql.DB")
	}

	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// 初始化熔断器
	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "database-" + cfg.Driver,
		Config: cbCfg,
	}, m)

	db := &DB{
		DB:      gormDB,
		cfg:     &cfg,
		breaker: cb,
		logger:  logger,
	}

	return db, nil
}

// Transaction 封装了带熔断保护的事务逻辑
func (db *DB) Transaction(fc func(tx *gorm.DB) error) error {
	_, err := db.breaker.Execute(func() (any, error) {
		err := db.DB.Transaction(fc)
		if err != nil {
			return nil, xerrors.Wrap(err, xerrors.ErrInternal, "transaction failed")
		}
		return nil, nil
	})
	return err
}

// RawDB 暴露原始 GORM 实例
func (db *DB) RawDB() *gorm.DB {
	return db.DB
}