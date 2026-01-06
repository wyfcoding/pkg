package sharding

import (
	"fmt"
	"sync"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/database"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"gorm.io/gorm"
)

var defaultManager *Manager

// Default 返回全局默认分片管理器实例
func Default() *Manager {
	return defaultManager
}

// SetDefault 设置全局默认分片管理器实例
func SetDefault(m *Manager) {
	defaultManager = m
}

// Manager 封装了水平分片（Sharding）的数据库访问逻辑，支持按 Key 路由。
type Manager struct {
	shards     map[int]*database.DB // 分片索引与数据库实例的映射
	shardCount int                  // 总分片数量
	mu         sync.RWMutex         // 保护分片映射的并发安全
}

// NewManager 初始化分片集群管理器。
// 参数 configs: 分片节点的配置列表。
func NewManager(configs []config.DatabaseConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) (*Manager, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no database configs provided")
	}

	shards := make(map[int]*database.DB)
	for i, cfg := range configs {
		db, err := database.NewDB(cfg, cbCfg, logger, m)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to shard %d: %w", i, err)
		}
		shards[i] = db
	}

	logger.Info("database sharding manager initialized", "shards_count", len(configs))

	return &Manager{
		shards:     shards,
		shardCount: len(configs),
	}, nil
}

// GetDB 根据分片键（通常是 userID）执行取模算法，返回对应的 GORM 数据库实例。
func (m *Manager) GetDB(key uint64) *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	shardIndex := int(key % uint64(m.shardCount))
	return m.shards[shardIndex].RawDB()
}

// GetAllDBs 返回集群中所有分片的实例列表，常用于跨分片的全量查询或批量统计。
func (m *Manager) GetAllDBs() []*gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dbs := make([]*gorm.DB, 0, m.shardCount)
	for i := 0; i < m.shardCount; i++ {
		dbs = append(dbs, m.shards[i].RawDB())
	}
	return dbs
}

// Close 优雅关闭集群中所有分片的数据库连接池。
func (m *Manager) Close() error {
	m.mu.Lock()         // 加写锁，以确保在关闭过程中不会有新的DB访问。
	defer m.mu.Unlock() // 确保函数退出时解锁。

	var errs []error // 收集关闭过程中可能遇到的所有错误。
	for i, db := range m.shards {
		// 获取GORM底层的 `*sql.DB` 实例，以便关闭连接池。
		sqlDB, err := db.RawDB().DB()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get sql db for shard %d: %w", i, err))
			continue
		}
		// 关闭数据库连接。
		if err := sqlDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close shard %d: %w", i, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close some shards: %v", errs) // 返回所有关闭失败的错误。
	}
	return nil
}
