// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
// 生成摘要:
// 1) 新增 HTTP/gRPC 审计中间件，统一采集动作、主体与结果信息。
// 2) 支持扩展资源标识与元数据，便于落库或异步投递。
// 假设:
// 1) 上游已注入租户/用户/请求 ID 等上下文字段。
package middleware

import (
	"context"
	"time"

	"github.com/wyfcoding/pkg/audit"
	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/tracing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuditOptions 定义 HTTP 审计中间件的可配置项。
type AuditOptions struct {
	Action     string
	Resource   string
	ResourceID func(c *gin.Context) string
	Metadata   func(c *gin.Context) map[string]string
	SkipPaths  []string
}

// AuditMiddleware 创建 HTTP 审计中间件。
func AuditMiddleware(writer audit.Writer, opts AuditOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		if writer == nil || pathSkipped(opts.SkipPaths, c.Request.URL.Path) {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		ctx := c.Request.Context()
		status := c.Writer.Status()
		result := audit.ResultSuccess
		if status < 200 || status >= 400 {
			result = audit.ResultFailure
		}

		event := audit.Event{
			Action:     auditAction(opts.Action, c.Request.Method, c.Request.URL.Path),
			Resource:   opts.Resource,
			ActorID:    contextx.GetUserID(ctx),
			TenantID:   contextx.GetTenantID(ctx),
			Result:     result,
			StatusCode: status,
			RequestID:  contextx.GetRequestID(ctx),
			TraceID:    tracing.GetTraceID(ctx),
			IP:         contextx.GetIP(ctx),
			UserAgent:  contextx.GetUserAgent(ctx),
			Timestamp:  time.Now(),
			Duration:   time.Since(start),
		}

		if opts.ResourceID != nil {
			event.ResourceID = opts.ResourceID(c)
		}
		if opts.Metadata != nil {
			event.Metadata = opts.Metadata(c)
		}
		if len(c.Errors) > 0 {
			event.Error = c.Errors.String()
		}

		if err := writer.Write(ctx, event); err != nil {
			logging.Error(ctx, "audit writer failed", "error", err)
		}
	}
}

// GRPCAuditOptions 定义 gRPC 审计拦截器的可配置项。
type GRPCAuditOptions struct {
	Action   string
	Resource string
	Metadata func(ctx context.Context, req any, info *grpc.UnaryServerInfo, err error) map[string]string
}

// GRPCAuditInterceptor 创建 gRPC 审计拦截器。
func GRPCAuditInterceptor(writer audit.Writer, opts GRPCAuditOptions) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if writer == nil {
			return handler(ctx, req)
		}

		start := time.Now()
		resp, err := handler(ctx, req)

		code := status.Code(err)
		result := audit.ResultSuccess
		if err != nil || code != codes.OK {
			result = audit.ResultFailure
		}

		event := audit.Event{
			Action:     auditAction(opts.Action, "grpc", info.FullMethod),
			Resource:   opts.Resource,
			ActorID:    contextx.GetUserID(ctx),
			TenantID:   contextx.GetTenantID(ctx),
			Result:     result,
			StatusCode: int(code),
			RequestID:  contextx.GetRequestID(ctx),
			TraceID:    tracing.GetTraceID(ctx),
			IP:         contextx.GetIP(ctx),
			UserAgent:  contextx.GetUserAgent(ctx),
			Timestamp:  time.Now(),
			Duration:   time.Since(start),
		}
		if err != nil {
			event.Error = err.Error()
		}
		if opts.Metadata != nil {
			event.Metadata = opts.Metadata(ctx, req, info, err)
		}

		if writeErr := writer.Write(ctx, event); writeErr != nil {
			logging.Error(ctx, "audit writer failed", "error", writeErr)
		}

		return resp, err
	}
}

func auditAction(action, method, path string) string {
	if action != "" {
		return action
	}
	return method + " " + path
}

func pathSkipped(paths []string, target string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}
