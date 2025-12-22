// Package grpcclient 提供了gRPC客户端连接的工厂模式，并集成了各种客户端拦截器，
// 用于实现服务间的通信的度量、熔断、限流、重试和日志记录等增强功能。
package grpcclient

import (
	"context"
	"time"

	"github.com/wyfcoding/pkg/logging" // 导入项目内定义的日志包

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc" // OpenTelemetry gRPC客户端追踪
	"golang.org/x/time/rate"                                                      // 基于令牌桶的限流器
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"                // gRPC状态码
	"google.golang.org/grpc/credentials/insecure" // 非安全传输凭证（生产环境应使用TLS）
	"google.golang.org/grpc/keepalive"            // gRPC心跳机制
	"google.golang.org/grpc/status"               // gRPC状态转换
)

// ClientConfig 简单的客户端配置
type ClientConfig struct {
	Target          string
	Timeout         int
	ConnTimeout     int
	RequestTimeout  int
	MaxRetries      int
	RetryDelay      int
	EnableKeepalive bool
}

// NewClient 简单的客户端创建函数，使用默认工厂
func NewClient(cfg ClientConfig) (*grpc.ClientConn, error) {
	factory := NewClientFactory(logging.Default())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return factory.NewClient(ctx, cfg.Target)
}

var (
	// grpcClientRequests 是一个Prometheus计数器，用于统计gRPC客户端请求的总数。
	// 标签包括方法名（method）、目标服务地址（target）和请求状态（status）。
	grpcClientRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grpc_client_requests_total",
			Help: "The total number of grpc client requests",
		},
		[]string{"method", "target", "status"},
	)
	// grpcClientDuration 是一个Prometheus直方图，用于记录gRPC客户端请求的持续时间。
	// 标签包括方法名（method）和目标服务地址（target）。
	grpcClientDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "grpc_client_duration_seconds",
			Help:    "The duration of grpc client requests",
			Buckets: prometheus.DefBuckets, // 使用Prometheus默认的直方图分桶
		},
		[]string{"method", "target"},
	)
)

// init 函数在包加载时自动执行，用于注册Prometheus指标。
func init() {
	prometheus.MustRegister(grpcClientRequests, grpcClientDuration)
}

// ClientFactory 负责创建和配置gRPC客户端连接。
// 它通过链式拦截器的方式，为每个客户端连接添加了多种横切关注点功能。
type ClientFactory struct {
	logger *logging.Logger // 用于客户端操作的日志记录器
}

// NewClientFactory 创建并返回一个新的 ClientFactory 实例。
func NewClientFactory(logger *logging.Logger) *ClientFactory {
	return &ClientFactory{
		logger: logger,
	}
}

// NewClient 创建一个新的gRPC客户端连接（ClientConn）。
// target: 目标gRPC服务的地址，例如 "localhost:50051"。
// opts: 额外的gRPC拨号选项，可以覆盖或补充默认选项。
// 该方法会初始化熔断器和限流器，并链式地应用多个客户端拦截器。
func (f *ClientFactory) NewClient(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	// 为每个客户端目标服务初始化一个熔断器实例。
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "grpc-client-" + target, // 熔断器名称
		MaxRequests: 0,                       // Half-Open状态下不限制请求数
		Interval:    0,                       // 不重置统计周期
		Timeout:     30 * time.Second,        // 从Open状态切换到Half-Open状态的超时时间
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// 熔断条件：如果总请求数 >= 10 且失败率 >= 60%，则触发熔断。
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 10 && failureRatio >= 0.6
		},
	})

	// 初始化一个令牌桶限流器，例如每秒允许1000个请求，桶容量为100。
	limiter := rate.NewLimiter(rate.Limit(1000), 100)

	// 定义gRPC连接的默认拨号选项。
	defaultOpts := []grpc.DialOption{
		// 配置传输凭证为非安全模式。在生产环境中，应使用TLS/SSL。
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// 配置gRPC心跳机制，用于检测连接的活跃性。
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                10 * time.Second, // 客户端发送心跳的间隔
			Timeout:             time.Second,      // 等待心跳响应的超时时间
			PermitWithoutStream: true,             // 即使没有活跃的流也允许发送心跳
		}),
		// 添加OpenTelemetry客户端追踪处理器，用于自动创建和传播追踪Span。
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		// 链式地应用多个一元客户端拦截器。拦截器会按照添加的顺序依次执行。
		grpc.WithChainUnaryInterceptor(
			f.metricsInterceptor(),          // 记录客户端请求指标
			f.circuitBreakerInterceptor(cb), // 实现客户端侧的熔断
			f.rateLimitInterceptor(limiter), // 实现客户端侧的限流
			f.retryInterceptor(),            // 实现客户端侧的请求重试
			f.loggingInterceptor(),          // 记录客户端请求日志
		),
	}

	// 将默认选项与传入的自定义选项合并。
	opts = append(defaultOpts, opts...)

	// 拨号连接到目标gRPC服务。
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		f.logger.ErrorContext(ctx, "failed to dial grpc target", "target", target, "error", err)
		return nil, err
	}

	return conn, nil
}

// metricsInterceptor 返回一个一元客户端拦截器，用于收集gRPC客户端请求的Prometheus指标。
func (f *ClientFactory) metricsInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		// 调用下一个拦截器或实际的RPC方法。
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start).Seconds() // 计算请求耗时。

		statusStr := "success"
		if err != nil {
			statusStr = status.Code(err).String() // 将gRPC错误码转换为字符串作为状态。
		}

		// 增加请求计数器。
		grpcClientRequests.WithLabelValues(method, cc.Target(), statusStr).Inc()
		// 记录请求持续时间。
		grpcClientDuration.WithLabelValues(method, cc.Target()).Observe(duration)

		return err
	}
}

// circuitBreakerInterceptor 返回一个一元客户端拦截器，用于在gRPC客户端侧应用熔断机制。
// 它使用gobreaker库来包装RPC调用。
func (f *ClientFactory) circuitBreakerInterceptor(cb *gobreaker.CircuitBreaker) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// 使用熔断器的Execute方法来执行实际的RPC调用。
		_, err := cb.Execute(func() (any, error) {
			// 在熔断器内部调用实际的gRPC方法。
			return nil, invoker(ctx, method, req, reply, cc, opts...)
		})
		// 如果熔断器开启，Execute会返回gobreaker.ErrOpenState，并最终被gRPC框架转换为gRPC错误码。
		return err
	}
}

// rateLimitInterceptor 返回一个一元客户端拦截器，用于在gRPC客户端侧实现请求限流。
// 它使用golang.org/x/time/rate库的令牌桶算法。
func (f *ClientFactory) rateLimitInterceptor(limiter *rate.Limiter) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Wait方法会阻塞直到令牌可用，或者上下文被取消。
		if err := limiter.Wait(ctx); err != nil {
			// 如果限流器等待失败（例如，上下文超时），则返回ResourceExhausted错误码。
			return status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}
		// 如果令牌可用，则继续调用实际的RPC方法。
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// retryInterceptor 返回一个一元客户端拦截器，用于对gRPC调用进行重试。
// 它只在特定的瞬时错误（如 Unavailable, DeadlineExceeded）时进行重试，并采用指数退避策略。
func (f *ClientFactory) retryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var err error
		for i := range 3 { // 最多重试3次
			err = invoker(ctx, method, req, reply, cc, opts...)
			if err == nil {
				return nil // RPC调用成功，返回nil。
			}

			// 获取gRPC错误码，判断是否为可重试错误。
			code := status.Code(err)
			// 仅当错误码是 Unavailable（服务不可用）或 DeadlineExceeded（截止时间已过）时才重试。
			if code != codes.Unavailable && code != codes.DeadlineExceeded {
				return err // 其他错误直接返回，不重试。
			}
			// 指数退避：每次重试前等待更长时间。
			time.Sleep(time.Duration(1<<i) * 100 * time.Millisecond) // 例如 100ms, 200ms, 400ms。
		}
		return err // 所有重试都失败，返回最后一次的错误。
	}
}

// loggingInterceptor 返回一个一元客户端拦截器，用于记录gRPC客户端调用的日志。
// 它会记录RPC方法、目标服务、耗时以及错误信息。
func (f *ClientFactory) loggingInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		// 调用下一个拦截器或实际的RPC方法。
		err := invoker(ctx, method, req, reply, cc, opts...)
		cost := time.Since(start) // 计算RPC调用耗时。

		if err != nil {
			// 如果调用失败，则以Error级别记录日志。
			f.logger.ErrorContext(ctx, "grpc client call failed",
				"method", method,
				"target", cc.Target(),
				"cost", cost,
				"error", err,
			)
		} else {
			// 如果调用成功，则以Debug级别记录日志。
			f.logger.DebugContext(ctx, "grpc client call success",
				"method", method,
				"target", cc.Target(),
				"cost", cost,
			)
		}

		return err
	}
}
