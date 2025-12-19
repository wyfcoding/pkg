package sharding

import (
	"fmt"
	"sync"

	"github.com/wyfcoding/pkg/config"    // 导入项目内定义的配置包。
	"github.com/wyfcoding/pkg/databases" // 导入项目内定义的数据库连接工厂。
	"github.com/wyfcoding/pkg/logging"   // 导入项目内定义的日志包。
	"gorm.io/gorm"                       // GORM ORM框架。
)

// Manager 结构体管理多个数据库分片。
// 它维护着每个分片对应的GORM DB实例，并提供了根据分片键获取对应DB实例的方法。
type Manager struct {
	shards     map[int]*gorm.DB // 存储分片索引到GORM DB实例的映射。
	shardCount int              // 分片的总数量。
	mu         sync.RWMutex     // 读写锁，用于保护分片map的并发访问。
}

// NewManager 创建并返回一个新的分片管理器实例。
// configs: 包含每个分片数据库配置的切片。
// logger: 用于数据库连接日志记录的日志器。
func NewManager(configs []config.DatabaseConfig, logger *logging.Logger) (*Manager, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("no database configs provided")
	}

	shards := make(map[int]*gorm.DB)
	// 遍历所有数据库配置，为每个配置创建一个数据库连接（分片）。
	for i, cfg := range configs {
		db, err := databases.NewDB(cfg, logger) // 调用 NewDB 函数建立数据库连接。
		if err != nil {
			return nil, fmt.Errorf("failed to connect to shard %d: %w", i, err)
		}
		shards[i] = db // 将创建的DB实例存储到shards map中。
	}

	return &Manager{
		shards:     shards,
		shardCount: len(configs), // 设置分片总数。
	}, nil
}

// GetDB 根据给定的分片键（例如，用户ID或订单ID）返回对应的数据库实例。
// 它使用哈希取模的方式将键映射到分片索引。
// key: 用于确定分片键的无符号64位整数（例如，UserID）。
// 返回对应分片的 `*gorm.DB` 实例。
func (m *Manager) GetDB(key uint64) *gorm.DB {
	m.mu.RLock()         // 加读锁，以确保读取shards map的线程安全。
	defer m.mu.RUnlock() // 确保函数退出时解锁。

	// 使用哈希取模算法确定分片索引。
	shardIndex := int(key % uint64(m.shardCount))
	return m.shards[shardIndex] // 返回对应的DB实例。
}

// Close 关闭所有数据库分片的连接。
// 它会遍历所有分片，并尝试关闭每个连接。
func (m *Manager) Close() error {
	m.mu.Lock()         // 加写锁，以确保在关闭过程中不会有新的DB访问。
	defer m.mu.Unlock() // 确保函数退出时解锁。

	var errs []error // 收集关闭过程中可能遇到的所有错误。
	for i, db := range m.shards {
		// 获取GORM底层的 `*sql.DB` 实例，以便关闭连接池。
		sqlDB, err := db.DB()
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
