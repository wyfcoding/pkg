// Package audit 提供审计事件的统一模型与写入器接口。
// 生成摘要:
// 1) 新增审计事件模型与标准结果枚举。
// 2) 提供日志写入与多路写入器，便于落库或投递消息队列。
// 假设:
// 1) 审计事件由业务自行补充资源标识与扩展字段。
package audit

import (
	"context"
	"time"

	"github.com/wyfcoding/pkg/logging"
)

// Result 表示审计事件的结果类型。
type Result string

const (
	// ResultSuccess 表示审计成功。
	ResultSuccess Result = "SUCCESS"
	// ResultFailure 表示审计失败。
	ResultFailure Result = "FAILURE"
)

// Event 定义通用审计事件结构。
type Event struct {
	Action     string            // 业务动作（例如 ORDER_CREATE）。
	Resource   string            // 资源类型（例如 order）。
	ResourceID string            // 资源 ID。
	ActorID    string            // 触发人 ID。
	TenantID   string            // 租户 ID。
	Result     Result            // 结果。
	StatusCode int               // HTTP/gRPC 状态码。
	RequestID  string            // 请求 ID。
	TraceID    string            // Trace ID。
	IP         string            // 客户端 IP。
	UserAgent  string            // 客户端 UA。
	Timestamp  time.Time         // 事件发生时间。
	Duration   time.Duration     // 处理耗时。
	Metadata   map[string]string // 扩展字段。
	Error      string            // 错误信息。
}

// Writer 定义审计事件写入器接口。
type Writer interface {
	Write(ctx context.Context, event Event) error
}

// LoggerWriter 使用统一日志系统写入审计事件。
type LoggerWriter struct {
	Logger *logging.Logger
}

// NewLoggerWriter 创建一个日志审计写入器。
func NewLoggerWriter(logger *logging.Logger) *LoggerWriter {
	return &LoggerWriter{Logger: logger}
}

// Write 输出结构化审计日志。
func (w *LoggerWriter) Write(ctx context.Context, event Event) error {
	logger := logging.Default()
	if w != nil && w.Logger != nil {
		logger = w.Logger
	}

	attrs := buildAuditAttrs(event)
	if event.Result == ResultFailure {
		logger.ErrorContext(ctx, "audit event", attrs...)
		return nil
	}

	logger.InfoContext(ctx, "audit event", attrs...)
	return nil
}

// FanoutWriter 将审计事件写入多个下游。
type FanoutWriter struct {
	writers []Writer
}

// NewFanoutWriter 创建一个多路写入器。
func NewFanoutWriter(writers ...Writer) *FanoutWriter {
	return &FanoutWriter{writers: writers}
}

// Write 依次写入所有下游写入器，并返回最后一个错误。
func (w *FanoutWriter) Write(ctx context.Context, event Event) error {
	var lastErr error
	for _, writer := range w.writers {
		if writer == nil {
			continue
		}
		if err := writer.Write(ctx, event); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func buildAuditAttrs(event Event) []any {
	attrs := make([]any, 0, 24)
	attrs = appendIf(attrs, "action", event.Action)
	attrs = appendIf(attrs, "resource", event.Resource)
	attrs = appendIf(attrs, "resource_id", event.ResourceID)
	attrs = appendIf(attrs, "actor_id", event.ActorID)
	attrs = appendIf(attrs, "tenant_id", event.TenantID)
	attrs = appendIf(attrs, "result", string(event.Result))
	if event.StatusCode != 0 {
		attrs = append(attrs, "status_code", event.StatusCode)
	}
	attrs = appendIf(attrs, "request_id", event.RequestID)
	attrs = appendIf(attrs, "trace_id", event.TraceID)
	attrs = appendIf(attrs, "client_ip", event.IP)
	attrs = appendIf(attrs, "user_agent", event.UserAgent)
	if !event.Timestamp.IsZero() {
		attrs = append(attrs, "timestamp", event.Timestamp)
	}
	if event.Duration > 0 {
		attrs = append(attrs, "duration", event.Duration)
	}
	if len(event.Metadata) > 0 {
		attrs = append(attrs, "metadata", event.Metadata)
	}
	attrs = appendIf(attrs, "error", event.Error)
	return attrs
}

func appendIf(attrs []any, key, value string) []any {
	if value == "" {
		return attrs
	}
	return append(attrs, key, value)
}
