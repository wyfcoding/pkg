package databases

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"gorm.io/plugin/opentelemetry/tracing"
)

var (
	dbPoolMaxOpen = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "db_pool_max_open",
		Help: "数据库连接池最大打开数",
	}, []string{"db_name"})
	dbPoolOpenConnections = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "db_pool_open_connections",
		Help: "数据库当前已建立连接数",
	}, []string{"db_name"})
	dbPoolInUse = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "db_pool_in_use",
		Help: "数据库当前正在使用的连接数",
	}, []string{"db_name"})
)

func init() {
	prometheus.MustRegister(dbPoolMaxOpen, dbPoolOpenConnections, dbPoolInUse)
}

// NewDB 创建具备全方位治理能力的 GORM 实例
func NewDB(cfg config.DatabaseConfig, logger *logging.Logger) (*gorm.DB, error) {
	gormLogger := logging.NewGormLogger(logger, cfg.SlowThreshold)

	gormConfig := &gorm.Config{
		Logger: gormLogger,
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		NowFunc: func() time.Time {
			return time.Now().Local()
		},
		// 【优化】：禁用默认事务，提升单条 SQL 的执行效率
		SkipDefaultTransaction: true,
		// 【性能】：开启预编译 SQL 缓存
		PrepareStmt: true,
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

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, err
	}

	// 1. 注入全链路追踪插件
	if err := db.Use(tracing.NewPlugin()); err != nil {
		return nil, fmt.Errorf("failed to use tracing plugin: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// 2. 连接池精细化配置
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// 3. 强一致性预热
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	// 4. 启动异步监控
	go collectDBStats(cfg.Driver, sqlDB)

	return db, nil
}

func collectDBStats(dbName string, sqlDB *sql.DB) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := sqlDB.Stats()
		dbPoolMaxOpen.WithLabelValues(dbName).Set(float64(stats.MaxOpenConnections))
		dbPoolOpenConnections.WithLabelValues(dbName).Set(float64(stats.OpenConnections))
		dbPoolInUse.WithLabelValues(dbName).Set(float64(stats.InUse))
	}
}

// WithTransaction 事务助手：全自动处理 Begin/Commit/Rollback/Panic
func WithTransaction(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(fn)
}
