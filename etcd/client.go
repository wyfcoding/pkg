package etcd

import (
	"context"
	"errors"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
)

const (
	defaultEtcdDialTimeout = 5 * time.Second
)

var defaultClient *clientv3.Client

// DefaultClient 返回默认 Etcd 客户端。
func DefaultClient() *clientv3.Client {
	return defaultClient
}

// SetDefaultClient 设置默认 Etcd 客户端。
func SetDefaultClient(client *clientv3.Client) {
	defaultClient = client
}

// NewClient 创建 Etcd 客户端并注入熔断保护。
func NewClient(cfg config.EtcdConfig, cbCfg config.CircuitBreakerConfig, logger *logging.Logger, m *metrics.Metrics) (*clientv3.Client, func(), error) {
	if len(cfg.Endpoints) == 0 {
		return nil, nil, errors.New("etcd endpoints is empty")
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = defaultEtcdDialTimeout
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:            cfg.Endpoints,
		Username:             cfg.Username,
		Password:             cfg.Password,
		DialTimeout:          cfg.DialTimeout,
		AutoSyncInterval:     cfg.AutoSync,
		DialKeepAliveTime:    5 * time.Second,
		DialKeepAliveTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "etcd",
		Config: cbCfg,
	}, m)

	healthCtx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()
	_, err = cb.Execute(func() (any, error) {
		_, err := client.Status(healthCtx, cfg.Endpoints[0])
		return struct{}{}, err
	})
	if err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	cleanup := func() {
		if closeErr := client.Close(); closeErr != nil {
			logger.Error("failed to close etcd client", "error", closeErr)
		}
	}

	return client, cleanup, nil
}
