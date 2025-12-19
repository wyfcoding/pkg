// Package databases 提供了数据库连接的初始化和配置功能，主要基于GORM ORM框架。
// 它集成了自定义日志、表命名策略和OpenTelemetry链路追踪。
package databases

import (
	"fmt"
	"time"

	"github.com/wyfcoding/pkg/config"  // 导入项目内定义的配置包
	"github.com/wyfcoding/pkg/logging" // 导入项目内定义的日志包

	"gorm.io/driver/clickhouse"            // GORM的ClickHouse驱动
	"gorm.io/driver/mysql"                 // GORM的MySQL驱动
	"gorm.io/driver/postgres"              // GORM的PostgreSQL驱动
	"gorm.io/gorm"                         // GORM核心库
	"gorm.io/gorm/schema"                  // GORM的Schema命名策略
	"gorm.io/plugin/opentelemetry/tracing" // GORM的OpenTelemetry追踪插件
)

// NewDB 创建并返回一个新的GORM数据库连接实例。
// cfg: 包含数据库连接详细信息和配置（如DSN、连接池设置、慢查询阈值等）的结构体。
// logger: 用于GORM日志输出的自定义日志记录器。
// 返回: 配置好的 `*gorm.DB` 实例和可能发生的错误。
func NewDB(cfg config.DatabaseConfig, logger *logging.Logger) (*gorm.DB, error) {
	// 使用自定义的GormLogger，它将GORM日志桥接到项目的统一日志系统。
	gormLogger := logging.NewGormLogger(logger, cfg.SlowThreshold)

	// 配置GORM全局设置。
	gormConfig := &gorm.Config{
		Logger: gormLogger, // 设置自定义日志器。
		// 配置命名策略，这里设置为使用单数形式的表名（例如，User struct对应'user'表）。
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		// 设置NowFunc，确保GORM在记录创建/更新时间时使用本地时间。
		NowFunc: func() time.Time {
			return time.Now().Local()
		},
	}

	var dialector gorm.Dialector
	// 根据配置的数据库驱动类型，选择对应的GORM驱动。
	switch cfg.Driver {
	case "mysql":
		dialector = mysql.Open(cfg.DSN) // DSN (Data Source Name) 包含连接MySQL所需的所有信息。
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	case "clickhouse":
		dialector = clickhouse.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", cfg.Driver)
	}

	// 尝试打开数据库连接。
	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, err
	}

	// 启用OpenTelemetry链路追踪插件。
	// 这将在每次数据库操作时自动创建Span，并记录SQL语句、耗时等信息。
	if err := db.Use(tracing.NewPlugin()); err != nil {
		return nil, fmt.Errorf("启用链路追踪插件失败: %w", err)
	}

	// 获取底层的sql.DB实例，用于配置连接池。
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// 配置数据库连接池参数，以优化性能和资源利用。
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)       // 最大空闲连接数。
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)       // 最大打开连接数。
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime) // 连接可被复用的最长时间。

	return db, nil
}
