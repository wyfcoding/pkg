package storage

import (
	"context"
	"io"
	"time"
)

// Storage 定义了对象存储的通用接口，支持多驱动扩展。
type Storage interface {
	// Upload 简单上传文件
	Upload(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) error

	// Download 下载文件
	Download(ctx context.Context, objectName string) (io.ReadCloser, error)

	// GetPresignedURL 获取带签名的临时访问地址 (用于 CDN 加速或私有访问)
	GetPresignedURL(ctx context.Context, objectName string, expiry time.Duration) (string, error)

	// Delete 删除文件
	Delete(ctx context.Context, objectName string) error

	// Multipart 相关的进阶操作
	InitiateMultipartUpload(ctx context.Context, objectName string, contentType string) (string, error)
	UploadPart(ctx context.Context, objectName, uploadID string, partNumber int, reader io.Reader, partSize int64) (string, error)
	CompleteMultipartUpload(ctx context.Context, objectName, uploadID string, parts []Part) error
	AbortMultipartUpload(ctx context.Context, objectName, uploadID string) error
}

// Part 定义分片信息
type Part struct {
	PartNumber int
	ETag       string
}
