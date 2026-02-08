package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/wyfcoding/pkg/database"

	"github.com/redis/go-redis/v9"
)

// Checker 定义健康检查函数原型。
type Checker func() error

// DBChecker 返回数据库健康检查函数。
func DBChecker(db *database.DB) Checker {
	return func() error {
		if db == nil {
			return errors.New("database is nil")
		}
		return db.Ping()
	}
}

// RedisChecker 返回 Redis 健康检查函数。
func RedisChecker(client redis.UniversalClient) Checker {
	return func() error {
		if client == nil {
			return errors.New("redis client is nil")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return client.Ping(ctx).Err()
	}
}

// HTTPChecker 返回 HTTP 依赖健康检查函数。
func HTTPChecker(url string, timeout time.Duration) Checker {
	return func() error {
		if url == "" {
			return errors.New("health check url is empty")
		}
		if timeout <= 0 {
			timeout = 2 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("http health check status: %d", resp.StatusCode)
		}
		return nil
	}
}
