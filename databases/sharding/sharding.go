package sharding

import (
	"fmt"
	"sync"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/databases"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"gorm.io/gorm"
)

// Manager 管理多个分片
type Manager struct {
	shards     map[int]*databases.DB
	shardCount int
	mu         sync.RWMutex
}

func NewManager(configs []config.DatabaseConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) (*Manager, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no database configs provided")
	}

	shards := make(map[int]*databases.DB)
	for i, cfg := range configs {
		db, err := databases.NewDB(cfg, cbCfg, logger, m)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to shard %d: %w", i, err)
		}
		shards[i] = db
	}

	return &Manager{
		shards:     shards,
		shardCount: len(configs),
	}, nil
}

func (m *Manager) GetDB(key uint64) *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	shardIndex := int(key % uint64(m.shardCount))
	return m.shards[shardIndex].RawDB()
}

// GetAllDBs 返回所有分片的 GORM DB 实例
func (m *Manager) GetAllDBs() []*gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dbs := make([]*gorm.DB, 0, m.shardCount)
	for i := 0; i < m.shardCount; i++ {
		dbs = append(dbs, m.shards[i].RawDB())
	}
	return dbs
}

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
