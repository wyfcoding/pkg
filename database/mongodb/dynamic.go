package mongodb

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"

	"go.mongodb.org/mongo-driver/mongo"
)

const (
	defaultMongoConnectTimeout = 5 * time.Second
	defaultMongoMaxPoolSize    = 100
)

type dynamicState struct {
	client   *mongo.Client
	cleanup  func()
	database string
}

// DynamicClient 提供支持热更新的 MongoDB 客户端包装器。
type DynamicClient struct {
	logger *logging.Logger
	mu     sync.Mutex
	state  atomic.Value
}

// NewDynamicClient 创建支持热更新的 MongoDB 客户端。
func NewDynamicClient(cfg Config, logger *logging.Logger) (*DynamicClient, error) {
	if logger == nil {
		logger = logging.Default()
	}
	client, cleanup, err := NewMongoClient(&cfg)
	if err != nil {
		return nil, err
	}
	d := &DynamicClient{logger: logger}
	d.state.Store(&dynamicState{
		client:   client,
		cleanup:  cleanup,
		database: cfg.Database,
	})
	return d, nil
}

// NewDynamicClientFromConfig 使用统一配置创建支持热更新的 MongoDB 客户端。
func NewDynamicClientFromConfig(cfg config.MongoDBConfig, logger *logging.Logger) (*DynamicClient, error) {
	return NewDynamicClient(buildMongoConfig(cfg), logger)
}

// UpdateConfig 使用最新配置替换 MongoDB 客户端。
func (d *DynamicClient) UpdateConfig(cfg Config) error {
	if d == nil {
		return errors.New("dynamic mongo client is nil")
	}
	logger := d.logger
	if logger == nil {
		logger = logging.Default()
	}
	client, cleanup, err := NewMongoClient(&cfg)
	if err != nil {
		return err
	}

	d.mu.Lock()
	old := d.load()
	d.state.Store(&dynamicState{
		client:   client,
		cleanup:  cleanup,
		database: cfg.Database,
	})
	d.mu.Unlock()

	if old != nil && old.cleanup != nil {
		old.cleanup()
	}

	logger.Info("mongodb client updated", "db", cfg.Database)

	return nil
}

// UpdateFromConfig 使用统一配置刷新 MongoDB 客户端。
func (d *DynamicClient) UpdateFromConfig(cfg config.MongoDBConfig) error {
	return d.UpdateConfig(buildMongoConfig(cfg))
}

// Client 返回当前 MongoDB 客户端实例。
func (d *DynamicClient) Client() *mongo.Client {
	state := d.load()
	if state == nil {
		return nil
	}
	return state.client
}

// Database 返回当前数据库名称。
func (d *DynamicClient) Database() string {
	state := d.load()
	if state == nil {
		return ""
	}
	return state.database
}

// Close 关闭当前 MongoDB 客户端连接。
func (d *DynamicClient) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	old := d.load()
	d.state.Store((*dynamicState)(nil))
	d.mu.Unlock()

	if old != nil && old.cleanup != nil {
		old.cleanup()
	}

	return nil
}

// RegisterReloadHook 注册 MongoDB 客户端热更新回调。
func RegisterReloadHook(client *DynamicClient, base Config) {
	if client == nil {
		return
	}
	config.RegisterReloadHook(func(updated *config.Config) {
		if updated == nil {
			return
		}
		next := base
		next.URI = updated.Data.MongoDB.URI
		next.Database = updated.Data.MongoDB.Database
		if next.ConnectTimeout <= 0 {
			next.ConnectTimeout = defaultMongoConnectTimeout
		}
		if next.MaxPoolSize == 0 {
			next.MaxPoolSize = defaultMongoMaxPoolSize
		}
		if err := client.UpdateConfig(next); err != nil {
			logger := client.logger
			if logger == nil {
				logger = logging.Default()
			}
			logger.Error("mongodb client reload failed", "error", err)
		}
	})
}

func (d *DynamicClient) load() *dynamicState {
	if d == nil {
		return nil
	}
	value := d.state.Load()
	if value == nil {
		return nil
	}
	return value.(*dynamicState)
}

func buildMongoConfig(cfg config.MongoDBConfig) Config {
	return Config{
		URI:            cfg.URI,
		Database:       cfg.Database,
		ConnectTimeout: defaultMongoConnectTimeout,
		MinPoolSize:    0,
		MaxPoolSize:    defaultMongoMaxPoolSize,
	}
}
