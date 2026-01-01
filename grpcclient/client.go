package grpcclient

import (
	"context"
	"time"

	"github.com/wyfcoding/pkg/breaker"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/utils"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

// ClientFactory 生产级的客户端工厂
type ClientFactory struct {
	logger  *logging.Logger
	metrics *metrics.Metrics
	cfg     config.CircuitBreakerConfig // 全局熔断配置
}

func NewClientFactory(logger *logging.Logger, m *metrics.Metrics, cfg config.CircuitBreakerConfig) *ClientFactory {
	return &ClientFactory{
		logger:  logger,
		metrics: m,
		cfg:     cfg,
	}
}

func (f *ClientFactory) NewClient(target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// 1. 使用项目标准熔断器
	cb := breaker.NewBreaker(breaker.Settings{
		Name:   "grpc-client-" + target,
		Config: f.cfg,
	}, f.metrics)

	// 2. 限流器
	limiter := rate.NewLimiter(rate.Limit(5000), 500)

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
			f.retryInterceptor(3),
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
		// 记录客户端指标，增加 target 标签以区分目标服务
		f.metrics.GrpcRequestsTotal.WithLabelValues("client", target+":"+method, st.Code().String()).Inc()
		f.metrics.GrpcRequestDuration.WithLabelValues("client", target+":"+method).Observe(duration)
		return err
	}
}

func (f *ClientFactory) circuitBreakerInterceptor(b *breaker.Breaker) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		_, err := b.Execute(func() (any, error) {
			err := invoker(ctx, method, req, reply, cc, opts...)
			return nil, err
		})
		if err == breaker.ErrServiceUnavailable {
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
		return utils.Retry(ctx, func() error {
			return invoker(ctx, method, req, reply, cc, opts...)
		}, utils.RetryConfig{MaxRetries: maxRetries})
	}
}
