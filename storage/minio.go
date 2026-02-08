package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/wyfcoding/pkg/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOClient 实现了 Storage 接口，是对接 MinIO 或 S3 兼容存储系统的具体驱动。
type MinIOClient struct {
	mu     sync.RWMutex
	client *minio.Client // 高级对象操作客户端。
	core   *minio.Core   // 低级分片接口客户端，用于精确控制分片逻辑。
	bucket string        // 当前驱动绑定的存储桶名称。
}

// NewMinIOClient 构造一个新的 MinIO 存储驱动。
// 流程：初始化客户端 -> 验证连接 -> 导出核心控制接口。
func NewMinIOClient(endpoint, accessKeyID, secretAccessKey, bucket string, useSSL bool) (*MinIOClient, error) {
	client, core, err := newMinioClients(endpoint, accessKeyID, secretAccessKey, useSSL)
	if err != nil {
		return nil, err
	}

	slog.Info("minio_client initialized", "endpoint", endpoint, "bucket", bucket)

	return &MinIOClient{
		client: client,
		core:   core,
		bucket: bucket,
	}, nil
}

// Upload 将数据流上传至绑定的存储桶。
func (c *MinIOClient) Upload(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) error {
	if c == nil {
		return errors.New("minio client is nil")
	}
	c.mu.RLock()
	client := c.client
	bucket := c.bucket
	c.mu.RUnlock()
	if client == nil {
		return errors.New("minio client not initialized")
	}
	start := time.Now()
	_, err := client.PutObject(ctx, bucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		slog.Error("minio upload failed", "object", objectName, "error", err)
		return err
	}
	slog.Debug("minio upload successful", "object", objectName, "duration", time.Since(start))
	return nil
}

func (c *MinIOClient) Download(ctx context.Context, objectName string) (io.ReadCloser, error) {
	if c == nil {
		return nil, errors.New("minio client is nil")
	}
	c.mu.RLock()
	client := c.client
	bucket := c.bucket
	c.mu.RUnlock()
	if client == nil {
		return nil, errors.New("minio client not initialized")
	}
	return client.GetObject(ctx, bucket, objectName, minio.GetObjectOptions{})
}

func (c *MinIOClient) Delete(ctx context.Context, objectName string) error {
	if c == nil {
		return errors.New("minio client is nil")
	}
	c.mu.RLock()
	client := c.client
	bucket := c.bucket
	c.mu.RUnlock()
	if client == nil {
		return errors.New("minio client not initialized")
	}
	return client.RemoveObject(ctx, bucket, objectName, minio.RemoveObjectOptions{})
}

// Exists 检查对象是否存在.
func (c *MinIOClient) Exists(ctx context.Context, objectName string) (bool, error) {
	if c == nil {
		return false, errors.New("minio client is nil")
	}
	c.mu.RLock()
	client := c.client
	bucket := c.bucket
	c.mu.RUnlock()
	if client == nil {
		return false, errors.New("minio client not initialized")
	}
	_, err := client.StatObject(ctx, bucket, objectName, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *MinIOClient) GetPresignedURL(ctx context.Context, objectName string, expiry time.Duration) (string, error) {
	if c == nil {
		return "", errors.New("minio client is nil")
	}
	c.mu.RLock()
	client := c.client
	bucket := c.bucket
	c.mu.RUnlock()
	if client == nil {
		return "", errors.New("minio client not initialized")
	}
	reqParams := make(url.Values)
	presignedURL, err := client.PresignedGetObject(ctx, bucket, objectName, expiry, reqParams)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

// --- 修复后的分片上传实现 ---

func (c *MinIOClient) InitiateMultipartUpload(ctx context.Context, objectName, contentType string) (string, error) {
	if c == nil {
		return "", errors.New("minio client is nil")
	}
	c.mu.RLock()
	core := c.core
	bucket := c.bucket
	c.mu.RUnlock()
	if core == nil {
		return "", errors.New("minio client not initialized")
	}
	// 使用 core.NewMultipartUpload 修复错误。
	uploadID, err := core.NewMultipartUpload(ctx, bucket, objectName, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to initiate multipart upload: %w", err)
	}
	return uploadID, nil
}

func (c *MinIOClient) UploadPart(ctx context.Context, objectName, uploadID string, partNumber int, reader io.Reader, partSize int64) (string, error) {
	if c == nil {
		return "", errors.New("minio client is nil")
	}
	c.mu.RLock()
	core := c.core
	bucket := c.bucket
	c.mu.RUnlock()
	if core == nil {
		return "", errors.New("minio client not initialized")
	}
	// 使用 core.PutObjectPart 修复错误。
	part, err := core.PutObjectPart(ctx, bucket, objectName, uploadID, partNumber, reader, partSize, minio.PutObjectPartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to upload part %d: %w", partNumber, err)
	}
	return part.ETag, nil
}

func (c *MinIOClient) CompleteMultipartUpload(ctx context.Context, objectName, uploadID string, parts []Part) error {
	if c == nil {
		return errors.New("minio client is nil")
	}
	c.mu.RLock()
	core := c.core
	bucket := c.bucket
	c.mu.RUnlock()
	if core == nil {
		return errors.New("minio client not initialized")
	}
	minioParts := make([]minio.CompletePart, 0, len(parts))
	for _, p := range parts {
		minioParts = append(minioParts, minio.CompletePart{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		})
	}

	// 使用 core.CompleteMultipartUpload 修复错误。
	_, err := core.CompleteMultipartUpload(ctx, bucket, objectName, uploadID, minioParts, minio.PutObjectOptions{})
	return err
}

func (c *MinIOClient) AbortMultipartUpload(ctx context.Context, objectName, uploadID string) error {
	if c == nil {
		return errors.New("minio client is nil")
	}
	c.mu.RLock()
	core := c.core
	bucket := c.bucket
	c.mu.RUnlock()
	if core == nil {
		return errors.New("minio client not initialized")
	}
	// 使用 core.AbortMultipartUpload 修复错误。
	return core.AbortMultipartUpload(ctx, bucket, objectName, uploadID)
}

func (c *MinIOClient) Close() error {
	return nil
}

// UpdateConfig 使用最新配置刷新 MinIO 客户端。
func (c *MinIOClient) UpdateConfig(cfg config.MinioConfig) error {
	if c == nil {
		return errors.New("minio client is nil")
	}
	client, core, err := newMinioClients(cfg.Endpoint, cfg.AccessKeyID, cfg.SecretAccessKey, cfg.UseSSL)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.client = client
	c.core = core
	c.bucket = cfg.BucketName
	c.mu.Unlock()

	slog.Info("minio client updated", "endpoint", cfg.Endpoint, "bucket", cfg.BucketName)

	return nil
}

// RegisterReloadHook 注册 MinIO 客户端热更新回调。
func RegisterReloadHook(client *MinIOClient) {
	if client == nil {
		return
	}
	config.RegisterReloadHook(func(updated *config.Config) {
		if updated == nil {
			return
		}
		if err := client.UpdateConfig(updated.Minio); err != nil {
			slog.Error("minio client reload failed", "error", err)
		}
	})
}

func newMinioClients(endpoint, accessKeyID, secretAccessKey string, useSSL bool) (*minio.Client, *minio.Core, error) {
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	}

	client, err := minio.New(endpoint, opts)
	if err != nil {
		slog.Error("failed to create minio client", "endpoint", endpoint, "error", err)
		return nil, nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	core, err := minio.NewCore(endpoint, opts)
	if err != nil {
		slog.Error("failed to create minio core client", "endpoint", endpoint, "error", err)
		return nil, nil, fmt.Errorf("failed to create minio core client: %w", err)
	}

	return client, core, nil
}
