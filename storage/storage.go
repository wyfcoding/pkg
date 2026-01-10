package storage

import (
	"context"
	"io"
	"time"
)

// Storage 定义了对象存储（Object Storage）的标准化接口。
// 抽象该接口旨在支持多云驱动切换（如 MinIO, AWS S3, 阿里云 OSS 等）.
// 并为上层业务提供一致的文件上传、下载及分片管理能力。
type Storage interface {
	// Upload 执行简单的小文件直传。
	Upload(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) error

	// Download 获取对象的读取流，调用方负责 Close 该流。
	Download(ctx context.Context, objectName string) (io.ReadCloser, error)

	// GetPresignedURL 生成一个受时效保护的带签名访问地址。
	// 常用于私有存储桶的下行分发，确保数据访问的安全受控。
	GetPresignedURL(ctx context.Context, objectName string, expiry time.Duration) (string, error)

	// Delete 永久删除指定的对象。
	Delete(ctx context.Context, objectName string) error

	// --- 进阶大文件分片上传 (Multipart Upload) 接口 --.

	// InitiateMultipartUpload 初始化一个分片上传任务，返回唯一的 uploadID。
	InitiateMultipartUpload(ctx context.Context, objectName string, contentType string) (string, error)

	// UploadPart 上传单个数据分片，返回该分片的 ETag（校验指纹）。
	UploadPart(ctx context.Context, objectName, uploadID string, partNumber int, reader io.Reader, partSize int64) (string, error)

	// CompleteMultipartUpload 完成所有分片的合并，正式生成对象。
	CompleteMultipartUpload(ctx context.Context, objectName, uploadID string, parts []Part) error

	// AbortMultipartUpload 终止分片上传任务，清理已上传的临时片段。
	AbortMultipartUpload(ctx context.Context, objectName, uploadID string) error
}

// Part 描述了分片上传中的单个片段元数据。
type Part struct {
	ETag       string // 分片的实体标签，由服务端返回用于后续校.
	PartNumber int    // 分片顺序编号（从 1 开始.
}
