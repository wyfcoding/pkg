// Package grpcclient 提供了统一治理能力的 gRPC 客户端工厂。
// 生成摘要:
// 1) 增加 Request/Trace ID 自动传递拦截器，确保跨服务链路一致。
// 2) metrics 为空时自动降级，避免空指针。
// 假设:
// 1) Request ID 通过 metadata 键 "x-request-id" 传递。
package grpcclient

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/idgen"
	"github.com/wyfcoding/pkg/limiter"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/retry"
	"github.com/wyfcoding/pkg/tracing"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const grpcRequestIDKey = "x-request-id"
const grpcTraceIDKey = "x-trace-id"
const grpcTenantIDKey = "x-tenant-id"
const grpcUserIDKey = "x-user-id"
const grpcScopesKey = "x-scopes"
const grpcRoleKey = "x-role"

// ClientFactory 是一个生产级的 gRPC 客户端工厂，集成了治理能力（限流、熔断、重试、监控、追踪）。
type ClientFactory struct {
	logger     *logging.Logger  // 日志记录器。
	metrics    *metrics.Metrics // 性能指标采集组件。
	cbConfig   atomic.Value
	retryCount atomic.Int64
	rateLimit  atomic.Int64
	rateBurst  atomic.Int64
	breakersMu sync.Mutex
	breakers   []*breaker.DynamicBreaker
	limitersMu sync.Mutex
	limiters   []*limiter.DynamicLimiter
}

// NewClientFactory 初始化 gRPC 客户端工厂。
func NewClientFactory(logger *logging.Logger, m *metrics.Metrics, cfg config.CircuitBreakerConfig) *ClientFactory {
	factory := &ClientFactory{
		logger:  logger,
		metrics: m,
	}
	factory.retryCount.Store(3)
	factory.rateLimit.Store(5000)
	factory.rateBurst.Store(500)
	factory.cbConfig.Store(cfg)
	return factory
}

// WithRetry 设置最大重试次数.
func (f *ClientFactory) WithRetry(count int) *ClientFactory {
	f.UpdateRetry(count)
	return f
}

// WithRateLimit 设置限流参数.
func (f *ClientFactory) WithRateLimit(limit, burst int) *ClientFactory {
	f.UpdateRateLimit(limit, burst)
	return f
}

// NewClient 创建并返回一个新的 gRPC 客户端连接（ClientConn）。
// 核心特性：
// 1. 自动注入负载均衡策略 (Round Robin)。
// 2. 注入 OpenTelemetry 追踪处理器。
// 3. 注入一元拦截器链：指标采集 -> 熔断保护 -> 频控拦截 -> 指数退避重试。
func (f *ClientFactory) NewClient(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	cb := breaker.NewDynamicBreaker("grpc-client-"+target, f.metrics, 0, 0)
	cb.Update(f.currentCircuitBreaker())
	f.registerBreaker(cb)

	dynamicLimiter := limiter.NewDynamicLimiter(nil)
	limit, burst := f.currentRateLimit()
	if limit > 0 {
		if burst <= 0 {
			burst = limit
		}
		dynamicLimiter.UpdateLocal(limit, burst)
	}
	f.registerLimiter(dynamicLimiter)

	defaultOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"weighted_rr":{}}]}`),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time: 20 * time.Second,
		}),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(
			f.requestIDInterceptor(),
			f.metricsInterceptor(target),
			f.circuitBreakerInterceptor(cb),
			f.rateLimitInterceptor(dynamicLimiter, target),
			f.retryInterceptor(&f.retryCount),
		),
	}

	opts = append(defaultOpts, opts...)
	return grpc.NewClient(target, opts...)
}

func (f *ClientFactory) metricsInterceptor(target string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if f.metrics == nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start).Seconds()
		st, _ := status.FromError(err)
		f.metrics.GRPCRequestsTotal.WithLabelValues("client", target+":"+method, st.Code().String()).Inc()
		f.metrics.GRPCRequestDuration.WithLabelValues("client", target+":"+method).Observe(duration)
		return err
	}
}

func (f *ClientFactory) requestIDInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		requestID := contextx.GetRequestID(ctx)
		if requestID == "" {
			requestID = idgen.GenIDString()
		}

		traceID := tracing.GetTraceID(ctx)
		tenantID := contextx.GetTenantID(ctx)
		userID := contextx.GetUserID(ctx)
		scopes := contextx.GetScopes(ctx)
		role := contextx.GetRole(ctx)

		if md, ok := metadata.FromOutgoingContext(ctx); ok {
			if len(md.Get(grpcRequestIDKey)) == 0 {
				ctx = metadata.AppendToOutgoingContext(ctx, grpcRequestIDKey, requestID)
			}
			if traceID != "" && len(md.Get(grpcTraceIDKey)) == 0 {
				ctx = metadata.AppendToOutgoingContext(ctx, grpcTraceIDKey, traceID)
			}
			if tenantID != "" && len(md.Get(grpcTenantIDKey)) == 0 {
				ctx = metadata.AppendToOutgoingContext(ctx, grpcTenantIDKey, tenantID)
			}
			if userID != "" && len(md.Get(grpcUserIDKey)) == 0 {
				ctx = metadata.AppendToOutgoingContext(ctx, grpcUserIDKey, userID)
			}
			if scopes != "" && len(md.Get(grpcScopesKey)) == 0 {
				ctx = metadata.AppendToOutgoingContext(ctx, grpcScopesKey, scopes)
			}
			if role != "" && len(md.Get(grpcRoleKey)) == 0 {
				ctx = metadata.AppendToOutgoingContext(ctx, grpcRoleKey, role)
			}
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		ctx = metadata.AppendToOutgoingContext(ctx, grpcRequestIDKey, requestID)
		if traceID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, grpcTraceIDKey, traceID)
		}
		if tenantID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, grpcTenantIDKey, tenantID)
		}
		if userID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, grpcUserIDKey, userID)
		}
		if scopes != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, grpcScopesKey, scopes)
		}
		if role != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, grpcRoleKey, role)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func (f *ClientFactory) circuitBreakerInterceptor(b *breaker.DynamicBreaker) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		_, err := b.Execute(func() (any, error) {
			callErr := invoker(ctx, method, req, reply, cc, opts...)
			return nil, callErr
		})
		if err != nil && errors.Is(err, breaker.ErrServiceUnavailable) {
			return status.Error(codes.Unavailable, "circuit breaker open")
		}
		return err
	}
}

func (f *ClientFactory) rateLimitInterceptor(l limiter.Limiter, target string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		allowed, err := l.Allow(ctx, target+":"+method)
		if err != nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		if !allowed {
			return status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func (f *ClientFactory) retryInterceptor(counter *atomic.Int64) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		count := 0
		if counter != nil {
			count = int(counter.Load())
		}
		if count <= 0 {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		return retry.Retry(ctx, func() error {
			return invoker(ctx, method, req, reply, cc, opts...)
		}, retry.Config{MaxRetries: count})
	}
}

// UpdateRetry 动态更新最大重试次数。
func (f *ClientFactory) UpdateRetry(count int) {
	if f == nil {
		return
	}
	f.retryCount.Store(int64(count))
}

// UpdateRateLimit 动态更新限流参数。
func (f *ClientFactory) UpdateRateLimit(limit, burst int) {
	if f == nil {
		return
	}
	f.rateLimit.Store(int64(limit))
	f.rateBurst.Store(int64(burst))
	f.updateLimiters(limit, burst)
}

// UpdateCircuitBreaker 动态更新熔断参数。
func (f *ClientFactory) UpdateCircuitBreaker(cfg config.CircuitBreakerConfig) {
	if f == nil {
		return
	}
	f.cbConfig.Store(cfg)
	f.updateBreakers(cfg)
}

// ClientFactoryOptions 定义动态更新的可选项。
type ClientFactoryOptions struct {
	RetryMax  int
	RateLimit int
	RateBurst int
	Breaker   config.CircuitBreakerConfig
}

// UpdateOptions 根据配置动态更新工厂参数。
func (f *ClientFactory) UpdateOptions(opts ClientFactoryOptions) {
	if f == nil {
		return
	}
	if opts.RetryMax != 0 {
		f.UpdateRetry(opts.RetryMax)
	}
	if opts.RateLimit != 0 || opts.RateBurst != 0 {
		f.UpdateRateLimit(opts.RateLimit, opts.RateBurst)
	}
	if opts.Breaker != (config.CircuitBreakerConfig{}) {
		f.UpdateCircuitBreaker(opts.Breaker)
	}
}

func (f *ClientFactory) currentCircuitBreaker() config.CircuitBreakerConfig {
	if f == nil {
		return config.CircuitBreakerConfig{}
	}
	val := f.cbConfig.Load()
	if val == nil {
		return config.CircuitBreakerConfig{}
	}
	return val.(config.CircuitBreakerConfig)
}

func (f *ClientFactory) currentRateLimit() (int, int) {
	if f == nil {
		return 0, 0
	}
	return int(f.rateLimit.Load()), int(f.rateBurst.Load())
}

func (f *ClientFactory) registerBreaker(b *breaker.DynamicBreaker) {
	f.breakersMu.Lock()
	f.breakers = append(f.breakers, b)
	f.breakersMu.Unlock()
}

func (f *ClientFactory) registerLimiter(l *limiter.DynamicLimiter) {
	f.limitersMu.Lock()
	f.limiters = append(f.limiters, l)
	f.limitersMu.Unlock()
}

func (f *ClientFactory) updateBreakers(cfg config.CircuitBreakerConfig) {
	f.breakersMu.Lock()
	items := make([]*breaker.DynamicBreaker, len(f.breakers))
	copy(items, f.breakers)
	f.breakersMu.Unlock()

	for _, item := range items {
		item.Update(cfg)
	}
}

func (f *ClientFactory) updateLimiters(limit, burst int) {
	if limit <= 0 {
		burst = 0
	}
	f.limitersMu.Lock()
	items := make([]*limiter.DynamicLimiter, len(f.limiters))
	copy(items, f.limiters)
	f.limitersMu.Unlock()

	for _, item := range items {
		if limit <= 0 {
			item.Update(nil)
			continue
		}
		adjusted := burst
		if adjusted <= 0 {
			adjusted = limit
		}
		item.UpdateLocal(limit, adjusted)
	}
}
