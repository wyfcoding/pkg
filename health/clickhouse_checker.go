package health

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

const defaultClickHouseHealthTimeout = 2 * time.Second

// ClickHouseChecker 返回 ClickHouse 依赖健康检查函数。
func ClickHouseChecker(addr, database, username, password string, timeout time.Duration) Checker {
	return func() error {
		if addr == "" {
			return errors.New("clickhouse addr is empty")
		}
		if timeout <= 0 {
			timeout = defaultClickHouseHealthTimeout
		}

		conn, err := clickhouse.Open(&clickhouse.Options{
			Addr: []string{addr},
			Auth: clickhouse.Auth{
				Database: database,
				Username: username,
				Password: password,
			},
			DialTimeout: timeout,
		})
		if err != nil {
			return fmt.Errorf("clickhouse connect failed: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := conn.Ping(ctx); err != nil {
			return fmt.Errorf("clickhouse ping failed: %w", err)
		}

		return nil
	}
}
