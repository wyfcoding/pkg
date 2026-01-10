package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOClient 实现了 Storage 接口，是对接 MinIO 或 S3 兼容存储系统的具体驱动。
type MinIOClient struct {
	client *minio.Client // 高级对象操作客户端。
	core   *minio.Core   // 低级分片接口客户端，用于精确控制分片逻辑。
	bucket string        // 当前驱动绑定的存储桶名称。
}

// NewMinIOClient 构造一个新的 MinIO 存储驱动。
// 流程：初始化客户端 -> 验证连接 -> 导出核心控制接口。
func NewMinIOClient(endpoint, accessKeyID, secretAccessKey, bucket string, useSSL bool) (*MinIOClient, error) {
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	}

	client, err := minio.New(endpoint, opts)
	if err != nil {
		slog.Error("failed to create minio client", "endpoint", endpoint, "error", err)
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	core, err := minio.NewCore(endpoint, opts)
	if err != nil {
		slog.Error("failed to create minio core client", "endpoint", endpoint, "error", err)
		return nil, fmt.Errorf("failed to create minio core client: %w", err)
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
	start := time.Now()
	_, err := c.client.PutObject(ctx, c.bucket, objectName, reader, size, minio.PutObjectOptions{
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
	return c.client.GetObject(ctx, c.bucket, objectName, minio.GetObjectOptions{})
}

func (c *MinIOClient) Delete(ctx context.Context, objectName string) error {
	return c.client.RemoveObject(ctx, c.bucket, objectName, minio.RemoveObjectOptions{})
}

func (c *MinIOClient) GetPresignedURL(ctx context.Context, objectName string, expiry time.Duration) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := c.client.PresignedGetObject(ctx, c.bucket, objectName, expiry, reqParams)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

// --- 修复后的分片上传实现 ---

func (c *MinIOClient) InitiateMultipartUpload(ctx context.Context, objectName, contentType string) (string, error) {
	// 使用 core.NewMultipartUpload 修复错误。
	uploadID, err := c.core.NewMultipartUpload(ctx, c.bucket, objectName, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to initiate multipart upload: %w", err)
	}
	return uploadID, nil
}

func (c *MinIOClient) UploadPart(ctx context.Context, objectName, uploadID string, partNumber int, reader io.Reader, partSize int64) (string, error) {
	// 使用 core.PutObjectPart 修复错误。
	part, err := c.core.PutObjectPart(ctx, c.bucket, objectName, uploadID, partNumber, reader, partSize, minio.PutObjectPartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to upload part %d: %w", partNumber, err)
	}
	return part.ETag, nil
}

func (c *MinIOClient) CompleteMultipartUpload(ctx context.Context, objectName, uploadID string, parts []Part) error {
	minioParts := make([]minio.CompletePart, 0, len(parts))
	for _, p := range parts {
		minioParts = append(minioParts, minio.CompletePart{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		})
	}

	// 使用 core.CompleteMultipartUpload 修复错误。
	_, err := c.core.CompleteMultipartUpload(ctx, c.bucket, objectName, uploadID, minioParts, minio.PutObjectOptions{})
	return err
}

func (c *MinIOClient) AbortMultipartUpload(ctx context.Context, objectName, uploadID string) error {
	// 使用 core.AbortMultipartUpload 修复错误。
	return c.core.AbortMultipartUpload(ctx, c.bucket, objectName, uploadID)
}

func (c *MinIOClient) Close() error {
	return nil
}
