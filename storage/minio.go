package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOClient 封装了 MinIO 客户端，提供对象存储服务的基础操作。
type MinIOClient struct {
	client *minio.Client // 底层MinIO客户端实例。
	bucket string        // 默认操作的存储桶名称。
}

// NewMinIOClient 创建并初始化一个新的 MinIO 客户端。
// endpoint: MinIO服务的地址，例如 "localhost:9000"。
// accessKeyID: 访问 MinIO 所需的Access Key。
// secretAccessKey: 访问 MinIO 所需的Secret Key。
// bucket: 客户端操作的默认存储桶名称。
// useSSL: 是否使用HTTPS协议连接MinIO。
func NewMinIOClient(endpoint, accessKeyID, secretAccessKey, bucket string, useSSL bool) (*MinIOClient, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""), // 使用静态凭证。
		Secure: useSSL,                                                    // 配置是否使用SSL。
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	return &MinIOClient{
		client: client,
		bucket: bucket,
	}, nil
}

// Upload 将文件上传到MinIO存储桶中。
// ctx: 上下文，用于控制操作的生命周期。
// objectName: 对象在存储桶中的名称（路径）。
// reader: 文件的内容来源。
// objectSize: 文件的大小（字节）。
// contentType: 文件的MIME类型，例如 "image/jpeg"。
func (c *MinIOClient) Upload(ctx context.Context, objectName string, reader io.Reader, objectSize int64, contentType string) error {
	_, err := c.client.PutObject(ctx, c.bucket, objectName, reader, objectSize, minio.PutObjectOptions{
		ContentType: contentType, // 设置对象的Content-Type。
	})
	if err != nil {
		return fmt.Errorf("failed to upload object: %w", err)
	}
	return nil
}

// Download 从MinIO存储桶下载文件。
// ctx: 上下文。
// objectName: 要下载的对象名称。
// 返回一个 `io.ReadCloser`，调用者需负责关闭。
func (c *MinIOClient) Download(ctx context.Context, objectName string) (io.ReadCloser, error) {
	obj, err := c.client.GetObject(ctx, c.bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	return obj, nil
}

// GetPresignedURL 生成一个用于访问对象的预签名URL。
// ctx: 上下文。
// objectName: 要生成URL的对象名称。
// expiry: URL的有效期，以秒为单位。
// 返回预签名URL字符串。
func (c *MinIOClient) GetPresignedURL(ctx context.Context, objectName string, expiry int64) (string, error) {
	reqParams := make(url.Values) // 可选的URL参数。
	// PresignedGetObject 生成一个GET请求的预签名URL。
	presignedURL, err := c.client.PresignedGetObject(ctx, c.bucket, objectName, time.Duration(expiry)*time.Second, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned url: %w", err)
	}
	return presignedURL.String(), nil
}
