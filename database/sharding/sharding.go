package sharding

import (
	"errors"
	"fmt"
	"sync"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/database"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/cast"
	"gorm.io/gorm"
)

var (
	defaultManager *Manager
	// ErrNoConfigs 未提供数据库配置.
	ErrNoConfigs = errors.New("no database configs provided")
	// ErrShardClose 部分分片关闭失败.
	ErrShardClose = errors.New("failed to close some shards")
)

// Default 返回全局默认分片管理器实例.
func Default() *Manager {
	return defaultManager
}

// SetDefault 设置全局默认分片管理器实例.
func SetDefault(m *Manager) {
	defaultManager = m
}

// Manager 封装了水平分片 (Sharding) 的数据库访问逻辑.
type Manager struct {
	shards     map[int]*database.DB
	mu         sync.RWMutex
	shardCount int
}

// NewManager 初始化分片集群管理器.
func NewManager(configs []config.DatabaseConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) (*Manager, error) {
	if len(configs) == 0 {
		return nil, ErrNoConfigs
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
		mu:         sync.RWMutex{},
	}, nil
}

// GetDB 根据分片键执行取模算法.
func (m *Manager) GetDB(key uint64) *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 安全转换：shardCount 始终为正且在初始化时受限于配置数量。
	// G115 Fix: Safe cast
	shardIndex := cast.Uint64ToIntSafe(key % cast.IntToUint64(m.shardCount))

	return m.shards[shardIndex].RawDB()
}

// GetAllDBs 返回集群中所有分片的实例列表.
func (m *Manager) GetAllDBs() []*gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dbs := make([]*gorm.DB, 0, m.shardCount)
	for i := range m.shardCount {
		dbs = append(dbs, m.shards[i].RawDB())
	}

	return dbs
}

// Close 优雅关闭集群中所有分片的数据库连接池.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for i, db := range m.shards {
		sqlDB, err := db.RawDB().DB()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get sql db for shard %d: %w", i, err))

			continue
		}

		if errClose := sqlDB.Close(); errClose != nil {
			errs = append(errs, fmt.Errorf("failed to close shard %d: %w", i, errClose))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %v", ErrShardClose, errs)
	}

	return nil
}
