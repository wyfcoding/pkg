// Package middleware 提供了 Gin 与 gRPC 的通用中间件实现。
// 生成摘要:
// 1) 新增 HTTP/gRPC 并发限制中间件，支持阻塞等待与快速失败。
// 2) 统一过载响应，避免过高并发拖垮服务。
// 假设:
// 1) 并发上限由业务服务按链路容量合理配置。
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/response"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ConcurrencyLimitOptions 定义并发控制的配置项。
type ConcurrencyLimitOptions struct {
	WaitTimeout time.Duration
}

// DynamicConcurrencyOptions 定义支持动态获取参数的配置项。
type DynamicConcurrencyOptions struct {
	WaitTimeout func() time.Duration
}

// ConcurrencyLimitWithLimiter 返回一个使用指定并发限流器的 Gin 中间件。
func ConcurrencyLimitWithLimiter(l limiter.ConcurrencyLimiter, opts ...ConcurrencyLimitOptions) gin.HandlerFunc {
	opt := ConcurrencyLimitOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	return func(c *gin.Context) {
		ctx := c.Request.Context()
		acquireCtx := ctx
		var cancel context.CancelFunc
		if opt.WaitTimeout > 0 {
			acquireCtx, cancel = context.WithTimeout(ctx, opt.WaitTimeout)
			defer cancel()
		}

		if err := l.Acquire(acquireCtx); err != nil {
			logging.Warn(ctx, "http concurrency limit exceeded", "error", err)
			response.ErrorWithStatus(c, http.StatusServiceUnavailable, "Service Busy", "concurrency limit exceeded")
			c.Abort()
			return
		}

		defer l.Release()
		c.Next()
	}
}

// ConcurrencyLimitWithDynamicOptions 返回一个支持动态配置的 Gin 并发限流中间件。
func ConcurrencyLimitWithDynamicOptions(l limiter.ConcurrencyLimiter, opt DynamicConcurrencyOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		acquireCtx := ctx
		var cancel context.CancelFunc
		waitTimeout := time.Duration(0)
		if opt.WaitTimeout != nil {
			waitTimeout = opt.WaitTimeout()
		}

		if waitTimeout > 0 {
			acquireCtx, cancel = context.WithTimeout(ctx, waitTimeout)
			defer cancel()
		}

		if err := l.Acquire(acquireCtx); err != nil {
			logging.Warn(ctx, "http concurrency limit exceeded", "error", err)
			response.ErrorWithStatus(c, http.StatusServiceUnavailable, "Service Busy", "concurrency limit exceeded")
			c.Abort()
			return
		}

		defer l.Release()
		c.Next()
	}
}

// NewConcurrencyLimitMiddleware 创建一个 Gin 并发限流中间件。
func NewConcurrencyLimitMiddleware(max int, waitTimeout time.Duration) gin.HandlerFunc {
	return ConcurrencyLimitWithLimiter(limiter.NewSemaphoreLimiter(max), ConcurrencyLimitOptions{WaitTimeout: waitTimeout})
}

// GRPCConcurrencyLimitOptions 定义 gRPC 并发控制配置项。
type GRPCConcurrencyLimitOptions struct {
	WaitTimeout time.Duration
}

// DynamicGRPCConcurrencyOptions 定义支持动态获取参数的 gRPC 配置项。
type DynamicGRPCConcurrencyOptions struct {
	WaitTimeout func() time.Duration
}

// GRPCConcurrencyLimitWithLimiter 返回一个使用指定并发限流器的 gRPC 拦截器。
func GRPCConcurrencyLimitWithLimiter(l limiter.ConcurrencyLimiter, opts ...GRPCConcurrencyLimitOptions) grpc.UnaryServerInterceptor {
	opt := GRPCConcurrencyLimitOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		acquireCtx := ctx
		var cancel context.CancelFunc
		if opt.WaitTimeout > 0 {
			acquireCtx, cancel = context.WithTimeout(ctx, opt.WaitTimeout)
			defer cancel()
		}

		if err := l.Acquire(acquireCtx); err != nil {
			logging.Warn(ctx, "grpc concurrency limit exceeded", "method", info.FullMethod, "error", err)
			return nil, status.Error(codes.ResourceExhausted, "concurrency limit exceeded")
		}

		defer l.Release()
		return handler(ctx, req)
	}
}

// GRPCConcurrencyLimitWithDynamicOptions 返回一个支持动态配置的 gRPC 并发限流拦截器。
func GRPCConcurrencyLimitWithDynamicOptions(l limiter.ConcurrencyLimiter, opt DynamicGRPCConcurrencyOptions) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		acquireCtx := ctx
		var cancel context.CancelFunc
		waitTimeout := time.Duration(0)
		if opt.WaitTimeout != nil {
			waitTimeout = opt.WaitTimeout()
		}

		if waitTimeout > 0 {
			acquireCtx, cancel = context.WithTimeout(ctx, waitTimeout)
			defer cancel()
		}

		if err := l.Acquire(acquireCtx); err != nil {
			logging.Warn(ctx, "grpc concurrency limit exceeded", "method", info.FullMethod, "error", err)
			return nil, status.Error(codes.ResourceExhausted, "concurrency limit exceeded")
		}

		defer l.Release()
		return handler(ctx, req)
	}
}

// NewGRPCConcurrencyLimitInterceptor 创建一个 gRPC 并发限流拦截器。
func NewGRPCConcurrencyLimitInterceptor(max int, waitTimeout time.Duration) grpc.UnaryServerInterceptor {
	return GRPCConcurrencyLimitWithLimiter(limiter.NewSemaphoreLimiter(max), GRPCConcurrencyLimitOptions{WaitTimeout: waitTimeout})
}
