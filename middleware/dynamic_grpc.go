package middleware

import (
	"context"
	"sync/atomic"

	"google.golang.org/grpc"
)

// DynamicUnaryInterceptor 提供可热更新的 gRPC 一元拦截器包装器。
type DynamicUnaryInterceptor struct {
	value atomic.Value
}

// NewDynamicUnaryInterceptor 创建动态一元拦截器封装器。
func NewDynamicUnaryInterceptor(initial grpc.UnaryServerInterceptor) *DynamicUnaryInterceptor {
	interceptor := &DynamicUnaryInterceptor{}
	if initial != nil {
		interceptor.value.Store(initial)
	}
	return interceptor
}

// Update 更新当前一元拦截器实现。
func (d *DynamicUnaryInterceptor) Update(interceptor grpc.UnaryServerInterceptor) {
	if d == nil {
		return
	}
	d.value.Store(interceptor)
}

// Interceptor 返回可注册到 gRPC 的动态一元拦截器。
func (d *DynamicUnaryInterceptor) Interceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if d == nil {
			return handler(ctx, req)
		}
		interceptor := d.load()
		if interceptor == nil {
			return handler(ctx, req)
		}
		return interceptor(ctx, req, info, handler)
	}
}

func (d *DynamicUnaryInterceptor) load() grpc.UnaryServerInterceptor {
	if d == nil {
		return nil
	}
	val := d.value.Load()
	if val == nil {
		return nil
	}
	return val.(grpc.UnaryServerInterceptor)
}
