package redis

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"

	"github.com/redis/go-redis/v9"
)

type dynamicState struct {
	client  redis.UniversalClient
	cleanup func()
}

// DynamicClient 提供支持热更新的 Redis 客户端包装器。
type DynamicClient struct {
	logger *logging.Logger
	mu     sync.Mutex
	state  atomic.Value
}

// NewDynamicClient 创建支持热更新的 Redis 客户端。
func NewDynamicClient(cfg config.RedisConfig, logger *logging.Logger) (*DynamicClient, error) {
	if logger == nil {
		logger = logging.Default()
	}
	client, cleanup, err := NewClient(&cfg, logger)
	if err != nil {
		return nil, err
	}
	d := &DynamicClient{logger: logger}
	d.state.Store(&dynamicState{
		client:  client,
		cleanup: cleanup,
	})
	return d, nil
}

// UpdateConfig 使用最新配置替换 Redis 客户端连接。
func (d *DynamicClient) UpdateConfig(cfg config.RedisConfig) error {
	if d == nil {
		return errors.New("dynamic redis client is nil")
	}
	logger := d.logger
	if logger == nil {
		logger = logging.Default()
	}
	client, cleanup, err := NewClient(&cfg, logger)
	if err != nil {
		return err
	}

	d.mu.Lock()
	old := d.load()
	d.state.Store(&dynamicState{
		client:  client,
		cleanup: cleanup,
	})
	d.mu.Unlock()

	if old != nil && old.cleanup != nil {
		old.cleanup()
	}

	logger.Info("redis client updated", "addrs", cfg.Addrs)

	return nil
}

// Client 返回当前 Redis 客户端实例。
func (d *DynamicClient) Client() redis.UniversalClient {
	state := d.load()
	if state == nil {
		return nil
	}
	return state.client
}

// Close 关闭当前 Redis 客户端连接。
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
