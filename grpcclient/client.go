package grpcclient

import (
	"context"
	"errors"
	"time"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/retry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

// ClientFactory 是一个生产级的 gRPC 客户端工厂，集成了治理能力（限流、熔断、重试、监控、追踪）。
type ClientFactory struct {
	logger     *logging.Logger             // 日志记录器。
	metrics    *metrics.Metrics            // 性能指标采集组件。
	cfg        config.CircuitBreakerConfig // 全局默认熔断配置。
	retryCount int                         // 最大重试次数。
	rateLimit  rate.Limit                  // 每秒请求限制。
	rateBurst  int                         // 突发请求限制。
}

// NewClientFactory 初始化 gRPC 客户端工厂。
func NewClientFactory(logger *logging.Logger, m *metrics.Metrics, cfg config.CircuitBreakerConfig) *ClientFactory {
	return &ClientFactory{
		logger:     logger,
		metrics:    m,
		cfg:        cfg,
		retryCount: 3,
		rateLimit:  rate.Limit(5000),
		rateBurst:  500,
	}
}

// WithRetry 设置最大重试次数.
func (f *ClientFactory) WithRetry(count int) *ClientFactory {
	f.retryCount = count
	return f
}

// WithRateLimit 设置限流参数.
func (f *ClientFactory) WithRateLimit(limit int, burst int) *ClientFactory {
	f.rateLimit = rate.Limit(limit)
	f.rateBurst = burst
	return f
}

// NewClient 创建并返回一个新的 gRPC 客户端连接（ClientConn）。
// 核心特性：
// 1. 自动注入负载均衡策略 (Round Robin)。
// 2. 注入 OpenTelemetry 追踪处理器。
// 3. 注入一元拦截器链：指标采集 -> 熔断保护 -> 频控拦截 -> 指数退避重试。
func (f *ClientFactory) NewClient(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "grpc-client-" + target,
		Config: f.cfg,
	}, f.metrics)

	limiter := rate.NewLimiter(f.rateLimit, f.rateBurst)

	defaultOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time: 20 * time.Second,
		}),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithChainUnaryInterceptor(
			f.metricsInterceptor(target),
			f.circuitBreakerInterceptor(cb),
			f.rateLimitInterceptor(limiter),
			f.retryInterceptor(f.retryCount),
		),
	}

	opts = append(defaultOpts, opts...)
	return grpc.NewClient(target, opts...)
}

func (f *ClientFactory) metricsInterceptor(target string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start).Seconds()
		st, _ := status.FromError(err)
		f.metrics.GRPCRequestsTotal.WithLabelValues("client", target+":"+method, st.Code().String()).Inc()
		f.metrics.GRPCRequestDuration.WithLabelValues("client", target+":"+method).Observe(duration)
		return err
	}
}

func (f *ClientFactory) circuitBreakerInterceptor(b *breaker.Breaker) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		_, err := b.Execute(func() (any, error) {
			err := invoker(ctx, method, req, reply, cc, opts...)
			return nil, err
		})
		if err != nil && errors.Is(err, breaker.ErrServiceUnavailable) {
			return status.Error(codes.Unavailable, "circuit breaker open")
		}
		return err
	}
}

func (f *ClientFactory) rateLimitInterceptor(limiter *rate.Limiter) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if !limiter.Allow() {
			return status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func (f *ClientFactory) retryInterceptor(maxRetries int) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return retry.Retry(ctx, func() error {
			return invoker(ctx, method, req, reply, cc, opts...)
		}, retry.Config{MaxRetries: maxRetries})
	}
}
