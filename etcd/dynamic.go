package etcd

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type dynamicState struct {
	client  *clientv3.Client
	cleanup func()
}

// DynamicClient 提供支持热更新的 Etcd 客户端包装器。
type DynamicClient struct {
	logger  *logging.Logger
	metrics *metrics.Metrics
	mu      sync.Mutex
	state   atomic.Value
}

// NewDynamicClient 创建支持热更新的 Etcd 客户端。
func NewDynamicClient(cfg config.EtcdConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) (*DynamicClient, error) {
	if logger == nil {
		logger = logging.Default()
	}
	client, cleanup, err := NewClient(cfg, cbCfg, logger, m)
	if err != nil {
		return nil, err
	}
	d := &DynamicClient{
		logger:  logger,
		metrics: m,
	}
	d.state.Store(&dynamicState{client: client, cleanup: cleanup})
	return d, nil
}

// UpdateConfig 使用最新配置刷新 Etcd 客户端。
func (d *DynamicClient) UpdateConfig(cfg config.EtcdConfig, cbCfg config.CircuitBreakerConfig) error {
	if d == nil {
		return errors.New("dynamic etcd client is nil")
	}
	logger := d.logger
	if logger == nil {
		logger = logging.Default()
	}
	client, cleanup, err := NewClient(cfg, cbCfg, logger, d.metrics)
	if err != nil {
		return err
	}

	d.mu.Lock()
	old := d.load()
	d.state.Store(&dynamicState{client: client, cleanup: cleanup})
	d.mu.Unlock()

	if old != nil && old.cleanup != nil {
		old.cleanup()
	}

	logger.Info("etcd client updated", "endpoints", cfg.Endpoints)

	return nil
}

// Client 返回当前 Etcd 客户端实例。
func (d *DynamicClient) Client() *clientv3.Client {
	state := d.load()
	if state == nil {
		return nil
	}
	return state.client
}

// Close 关闭当前 Etcd 客户端。
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

// RegisterReloadHook 注册 Etcd 客户端热更新回调。
func RegisterReloadHook(client *DynamicClient) {
	if client == nil {
		return
	}
	config.RegisterReloadHook(func(updated *config.Config) {
		if updated == nil {
			return
		}
		if err := client.UpdateConfig(updated.Data.Etcd, updated.CircuitBreaker); err != nil {
			logger := client.logger
			if logger == nil {
				logger = logging.Default()
			}
			logger.Error("etcd client reload failed", "error", err)
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
