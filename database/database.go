package database

import (
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

var defaultDB *DB

// DB 封装了 GORM 实例，集成了单例管理、熔断保护、指标监控以及分布式链路追踪。
type DB struct {
	*gorm.DB                        // 嵌入原生 GORM 数据库实例
	cfg      *config.DatabaseConfig // 数据库配置信息
	breaker  *breaker.Breaker       // 熔断器，保护数据库免受过载
	logger   *logging.Logger        // 日志记录器
}

// Default 返回全局默认的数据库连接实例。
func Default() *DB {
	return defaultDB
}

// SetDefault 设置全局默认数据库连接，通常在应用启动引导时调用。
func SetDefault(db *DB) {
	defaultDB = db
}

// NewDB 初始化并返回一个功能增强的数据库连接封装。
// 流程：选择驱动 -> 开启 GORM -> 注入 OTEL 插件 -> 配置连接池 -> 初始化熔断器。
func NewDB(cfg config.DatabaseConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) (*DB, error) {
	// ... (驱动选择与初始化逻辑) ...
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
		Logger:      logging.NewGormLogger(logger, 200*time.Millisecond),
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
