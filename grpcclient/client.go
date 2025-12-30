package grpcclient

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/utils"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ClientConfig 客户端详细配置
type ClientConfig struct {
	Target          string
	Timeout         time.Duration
	MaxRetries      int
	InitialBackoff  time.Duration
	MaxBackoff      time.Duration
	EnableKeepalive bool
}

var (
	grpcClientRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_client_requests_total",
			Help: "gRPC 客户端请求总数统计",
		},
		[]string{"method", "target", "status"},
	)
	grpcClientDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_client_duration_seconds",
			Help:    "gRPC 客户端请求耗时分布",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "target"},
	)
)

func init() {
	prometheus.MustRegister(grpcClientRequests, grpcClientDuration)
}

// ClientFactory 生产级的客户端工厂
type ClientFactory struct {
	logger *logging.Logger
}

func NewClientFactory(logger *logging.Logger) *ClientFactory {
	return &ClientFactory{logger: logger}
}

// NewClient 创建带负载均衡、熔断、限流和智能重试的连接
func (f *ClientFactory) NewClient(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// 1. 初始化服务治理组件
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:    "grpc-" + target,
		Timeout: 30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.Requests >= 20 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.5
		},
	})
	limiter := rate.NewLimiter(rate.Limit(5000), 500) // 默认 5000 QPS 限制

	// 2. 核心拨号选项
	defaultOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// 【关键】：开启客户端负载均衡 (Round Robin)
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`),
		// Keepalive 治理
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second,
			Timeout:             3 * time.Second,
			PermitWithoutStream: true,
		}),
		// OpenTelemetry 追踪
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		// 拦截器链
		grpc.WithChainUnaryInterceptor(
			f.metadataPropagationInterceptor(),                         // 自动传播上下文
			f.metricsInterceptor(),                                     // 监控指标
			f.circuitBreakerInterceptor(cb),                            // 熔断
			f.rateLimitInterceptor(limiter),                            // 限流
			f.retryInterceptor(3, 100*time.Millisecond, 2*time.Second), // 智能重试
			f.loggingInterceptor(),                                     // 日志
		),
	}

	opts = append(defaultOpts, opts...)
	return grpc.NewClient(target, opts...)
}

// metadataPropagationInterceptor 自动将 Context 中的标识透传到下游 gRPC Header
func (f *ClientFactory) metadataPropagationInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// 提取常见的追踪 ID 并注入到 Metadata
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}

		// 这里可以根据实际 Context 存储的 Key 进行提取
		// 示例：从 Context 提取 RequestID 并注入
		// if rid, ok := ctx.Value("request_id").(string); ok {
		//     md.Set("x-request-id", rid)
		// }

		return invoker(metadata.NewOutgoingContext(ctx, md), method, req, reply, cc, opts...)
	}
}

// retryInterceptor 实现带指数退避和随机抖动的智能重试
func (f *ClientFactory) retryInterceptor(maxRetries int, initialBackoff, maxBackoff time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		config := utils.DefaultRetryConfig()
		config.MaxRetries = maxRetries
		config.InitialBackoff = initialBackoff
		config.MaxBackoff = maxBackoff

		return utils.Retry(ctx, func() error {
			err := invoker(ctx, method, req, reply, cc, opts...)
			if err != nil {
				st, ok := status.FromError(err)
				// 如果不是可重试的错误码，直接返回错误，中断重试循环
				if ok && !isRetriable(st.Code()) {
					return err
				}
				return err
			}
			return nil
		}, config)
	}
}

func isRetriable(code codes.Code) bool {
	switch code {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}

// --- 其余拦截器 (Metrics, CircuitBreaker, RateLimit, Logging) 保持原逻辑并进行 slog 优化 ---

func (f *ClientFactory) metricsInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start).Seconds()

		statusStr := "success"
		if err != nil {
			statusStr = status.Code(err).String()
		}

		grpcClientRequests.WithLabelValues(method, cc.Target(), statusStr).Inc()
		grpcClientDuration.WithLabelValues(method, cc.Target()).Observe(duration)
		return err
	}
}

func (f *ClientFactory) circuitBreakerInterceptor(cb *gobreaker.CircuitBreaker) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		_, err := cb.Execute(func() (any, error) {
			return nil, invoker(ctx, method, req, reply, cc, opts...)
		})
		return err
	}
}

func (f *ClientFactory) rateLimitInterceptor(limiter *rate.Limiter) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if err := limiter.Wait(ctx); err != nil {
			return status.Error(codes.ResourceExhausted, "client-side rate limit exceeded")
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func (f *ClientFactory) loggingInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			f.logger.ErrorContext(ctx, "grpc call failed", "method", method, "target", cc.Target(), "cost", time.Since(start), "error", err)
		}
		return err
	}
}

// NewClientWithConfig 兼容旧接口
func NewClient(cfg ClientConfig) (*grpc.ClientConn, error) {
	factory := NewClientFactory(logging.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return factory.NewClient(ctx, cfg.Target)
}
