package health

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultMongoHealthTimeout = 2 * time.Second

// MongoChecker 返回 MongoDB 依赖健康检查函数。
func MongoChecker(uri string, timeout time.Duration) Checker {
	return func() error {
		if uri == "" {
			return errors.New("mongodb uri is empty")
		}
		if timeout <= 0 {
			timeout = defaultMongoHealthTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
		if err != nil {
			return fmt.Errorf("mongodb connect failed: %w", err)
		}
		defer func() {
			_ = client.Disconnect(context.Background())
		}()

		if err := client.Ping(ctx, nil); err != nil {
			return fmt.Errorf("mongodb ping failed: %w", err)
		}

		return nil
	}
}
