package health

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

const defaultNeo4jHealthTimeout = 2 * time.Second

// Neo4jChecker 返回 Neo4j 依赖健康检查函数。
func Neo4jChecker(uri, username, password string, timeout time.Duration) Checker {
	return func() error {
		if uri == "" {
			return errors.New("neo4j uri is empty")
		}
		if timeout <= 0 {
			timeout = defaultNeo4jHealthTimeout
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
		if err != nil {
			return fmt.Errorf("neo4j driver init failed: %w", err)
		}
		defer func() {
			_ = driver.Close(ctx)
		}()

		if err := driver.VerifyConnectivity(ctx); err != nil {
			return fmt.Errorf("neo4j connectivity check failed: %w", err)
		}

		return nil
	}
}
