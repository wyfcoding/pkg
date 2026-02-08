package health

import (
	"context"
	"errors"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const defaultEtcdHealthTimeout = 2 * time.Second

// EtcdChecker 返回 Etcd 依赖健康检查函数。
func EtcdChecker(cfg clientv3.Config, timeout time.Duration) Checker {
	return func() error {
		if len(cfg.Endpoints) == 0 {
			return errors.New("etcd endpoints is empty")
		}
		if timeout <= 0 {
			timeout = defaultEtcdHealthTimeout
		}

		client, err := clientv3.New(cfg)
		if err != nil {
			return fmt.Errorf("etcd client init failed: %w", err)
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		_, err = client.Status(ctx, cfg.Endpoints[0])
		if err != nil {
			return fmt.Errorf("etcd status failed: %w", err)
		}

		return nil
	}
}
