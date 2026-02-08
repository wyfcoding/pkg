package etcd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

var (
	// ErrLeaderConfig 配置非法。
	ErrLeaderConfig = errors.New("invalid leader election config")
)

// LeaderConfig 定义 Etcd 领导选举参数。
type LeaderConfig struct {
	Key        string
	TTL        int
	RetryDelay time.Duration
}

// LeaderElector 基于 Etcd 的领导选举实现。
type LeaderElector struct {
	client  *clientv3.Client
	logger  *slog.Logger
	cfg     LeaderConfig
	running int32
}

// NewLeaderElector 创建 Etcd 领导选举器。
func NewLeaderElector(client *clientv3.Client, logger *slog.Logger, cfg LeaderConfig) (*LeaderElector, error) {
	if client == nil || cfg.Key == "" || cfg.TTL <= 0 {
		return nil, ErrLeaderConfig
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 2 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &LeaderElector{
		client: client,
		logger: logger.With("module", "etcd_leader"),
		cfg:    cfg,
	}, nil
}

// Campaign 参与竞选，成功则执行 callback，直到失去领导权或 ctx 结束。
func (e *LeaderElector) Campaign(ctx context.Context, callback func(ctx context.Context)) {
	if !atomic.CompareAndSwapInt32(&e.running, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&e.running, 0)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		session, err := concurrency.NewSession(e.client, concurrency.WithTTL(e.cfg.TTL))
		if err != nil {
			e.logger.Error("failed to create etcd session", "error", err)
			time.Sleep(e.cfg.RetryDelay)
			continue
		}

		election := concurrency.NewElection(session, e.cfg.Key)
		e.logger.Info("leader campaign started", "key", e.cfg.Key)

		if err := election.Campaign(ctx, e.cfg.Key); err != nil {
			_ = session.Close()
			e.logger.Error("leader campaign failed", "error", err)
			time.Sleep(e.cfg.RetryDelay)
			continue
		}

		e.logger.Info("leader elected", "key", e.cfg.Key)
		callback(ctx)

		resignCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if resignErr := election.Resign(resignCtx); resignErr != nil {
			e.logger.Error("leader resign failed", "error", resignErr)
		}
		cancel()
		_ = session.Close()
		e.logger.Info("leader resigned", "key", e.cfg.Key)
	}
}

// CurrentLeader 返回当前 leader 的值。
func (e *LeaderElector) CurrentLeader(ctx context.Context) (string, error) {
	if e.client == nil || e.cfg.Key == "" {
		return "", ErrLeaderConfig
	}
	session, err := concurrency.NewSession(e.client, concurrency.WithTTL(e.cfg.TTL))
	if err != nil {
		return "", fmt.Errorf("create session failed: %w", err)
	}
	defer session.Close()

	election := concurrency.NewElection(session, e.cfg.Key)
	resp, err := election.Leader(ctx)
	if err != nil {
		return "", err
	}
	return string(resp.Kvs[0].Value), nil
}
