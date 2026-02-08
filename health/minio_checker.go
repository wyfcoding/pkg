package health

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const defaultMinioHealthTimeout = 2 * time.Second

// MinioChecker 返回 MinIO 依赖健康检查函数。
func MinioChecker(endpoint, accessKey, secretKey string, useSSL bool, timeout time.Duration) Checker {
	return func() error {
		if endpoint == "" {
			return errors.New("minio endpoint is empty")
		}
		if timeout <= 0 {
			timeout = defaultMinioHealthTimeout
		}

		client, err := minio.New(endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
			Secure: useSSL,
		})
		if err != nil {
			return fmt.Errorf("minio client init failed: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if _, err := client.ListBuckets(ctx); err != nil {
			return fmt.Errorf("minio list buckets failed: %w", err)
		}

		return nil
	}
}
