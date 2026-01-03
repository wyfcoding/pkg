package app

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/middleware"
	"github.com/wyfcoding/pkg/response"
	"github.com/wyfcoding/pkg/server"
	"github.com/wyfcoding/pkg/tracing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

// Builder 提供了构建 App 的灵活方式。
type Builder struct {
	serviceName    string   // 服务名称
	configInstance any      // 配置实例
	appOpts        []Option // 应用程序选项

	initService  any // 服务初始化函数
	registerGRPC any // gRPC 注册函数
	registerGin  any // Gin 注册函数

	metricsPort string // Metrics 端口

	// grpcInterceptors 用于收集gRPC一元拦截器。
	grpcInterceptors []grpc.UnaryServerInterceptor
	// ginMiddleware 用于收集Gin中间件。
	ginMiddleware []gin.HandlerFunc
}

// NewBuilder 创建一个新的应用构建器。
func NewBuilder(serviceName string) *Builder {
	return &Builder{serviceName: serviceName}
}

// WithConfig 设置配置实例。
// conf 应该是一个结构体的指针，用于加载配置。
func (b *Builder) WithConfig(conf any) *Builder {
	b.configInstance = conf
	return b
}

// WithGRPC 注册 gRPC 服务器的创建逻辑。
// register 函数负责将具体的gRPC服务注册到 `*grpc.Server` 实例中。
func (b *Builder) WithGRPC(register func(*grpc.Server, any)) *Builder {
	b.registerGRPC = register
	return b
}

// WithGin 注册 Gin 服务器的创建逻辑。
// register 函数负责将HTTP路由注册到 `*gin.Engine` 实例中。
func (b *Builder) WithGin(register func(*gin.Engine, any)) *Builder {
	b.registerGin = register
	return b
}

// WithService 注册服务的初始化逻辑。
// init 函数接收配置和Metrics实例，并返回服务实例、清理函数和错误。
func (b *Builder) WithService(init func(any, *metrics.Metrics) (any, func(), error)) *Builder {
	b.initService = init
	return b
}

// WithMetrics 在指定端口上启用 Prometheus 指标服务器。
func (b *Builder) WithMetrics(port string) *Builder {
	b.metricsPort = port
	return b
}

// WithGRPCInterceptor 添加一个或多个 gRPC 一元拦截器。
// 这些拦截器将在gRPC服务器创建时被链式应用。
func (b *Builder) WithGRPCInterceptor(interceptors ...grpc.UnaryServerInterceptor) *Builder {
	b.grpcInterceptors = append(b.grpcInterceptors, interceptors...)
	return b
}

// WithGinMiddleware 添加一个或多个 Gin 中间件。
// 这些中间件将在Gin引擎创建时被应用。
func (b *Builder) WithGinMiddleware(middleware ...gin.HandlerFunc) *Builder {
	b.ginMiddleware = append(b.ginMiddleware, middleware...)
	return b
}

// Build 构建最终的 App 实例。
// 它负责加载配置、初始化日志、Metrics、服务实例，并创建和注册gRPC和Gin服务器。
func (b *Builder) Build() *App {
	// 1. 加载配置：优先从命令行参数 -conf 获取，默认为相对路径下的 config.toml。
	configPath := fmt.Sprintf("./configs/%s/config.toml", b.serviceName)
	var flagConfigPath string
	flag.StringVar(&flagConfigPath, "conf", configPath, "path to config file")
	flag.Parse()

	if err := config.Load(flagConfigPath, b.configInstance); err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	// 提取 config.Config 核心配置信息。
	// 为了保持 Builder 的通用性，我们支持直接传入 config.Config 指针或者包含 Config 字段的结构体。
	var cfg config.Config
	// 尝试直接断言
	if c, ok := b.configInstance.(*config.Config); ok {
		cfg = *c
	} else {
		// 使用反射从自定义配置结构体中提取嵌入的 Config 结构。
		val := reflect.ValueOf(b.configInstance).Elem()
		// 检查是否是结构体
		if val.Kind() != reflect.Struct {
			panic("config instance must be a pointer to a struct")
		}
		// 查找名为 "Config" 的字段
		cfgField := val.FieldByName("Config")
		if cfgField.IsValid() && cfgField.Type() == reflect.TypeFor[config.Config]() {
			cfg = cfgField.Interface().(config.Config)
		} else {
			// 如果找不到名为 Config 的字段，或者类型不对，尝试查找是否嵌入了 config.Config（匿名主要字段）。
			found := false
			for i := 0; i < val.NumField(); i++ {
				field := val.Type().Field(i)
				if field.Anonymous && field.Type == reflect.TypeFor[config.Config]() {
					cfg = val.Field(i).Interface().(config.Config)
					found = true
					break
				}
			}
			if !found {
				panic("config instance must be *config.Config, or contain a field/embedded field of type config.Config named 'Config'")
			}
		}
	}

	// 2. 初始化日志：根据配置决定输出到控制台还是文件，并配置日志切割策略。
	logConfig := logging.Config{
		Service:    b.serviceName,
		Module:     "app",
		Level:      cfg.Log.Level,
		File:       cfg.Log.File,
		MaxSize:    cfg.Log.MaxSize,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAge:     cfg.Log.MaxAge,
		Compress:   cfg.Log.Compress,
	}
	// 如果配置指定 Output 为文件但没有提供路径，或者 Output 不是 "file"，则确保不输出到文件。
	if cfg.Log.Output != "file" {
		logConfig.File = ""
	}

	logger := logging.NewFromConfig(logConfig)
	// 设置为全局默认 Logger，使得 slog.Info 等可以直接使用配置好的 Logger。
	slog.SetDefault(logger.Logger)

	// 3. 初始化分布式追踪 (OpenTelemetry)：如果启用，则配置 OTLP 导出器并注册清理函数。
	if cfg.Tracing.Enabled {
		shutdown, err := tracing.InitTracer(cfg.Tracing)
		if err != nil {
			logger.Logger.Error("Failed to initialize tracer", "error", err)
		} else {
			// 注册追踪系统的关闭回调，确保缓冲区数据在应用退出前冲刷。
			b.appOpts = append(b.appOpts, WithCleanup(func() {
				if err := shutdown(context.Background()); err != nil {
					logger.Logger.Error("Failed to shutdown tracer", "error", err)
				}
			}))
			logger.Logger.Info("Tracer initialized", "endpoint", cfg.Tracing.OTLPEndpoint)

			// 自动添加 Gin 追踪中间件，确保每个入站 HTTP 请求都有 trace_id。
			b.ginMiddleware = append([]gin.HandlerFunc{middleware.TracingMiddleware(b.serviceName)}, b.ginMiddleware...)
		}
	}

	// 4. 初始化指标收集 (Prometheus)：在指定端口开启 HTTP 服务供 Prometheus 抓取数据。
	var metricsInstance *metrics.Metrics
	metricsPort := b.metricsPort
	if metricsPort == "" && cfg.Metrics.Enabled {
		metricsPort = cfg.Metrics.Port
	}

	if metricsPort != "" {
		metricsInstance = metrics.NewMetrics(b.serviceName)
		metricsCleanup := metricsInstance.ExposeHttp(metricsPort)
		b.appOpts = append(b.appOpts, WithCleanup(metricsCleanup))
	} else {
		// 即使不暴露 HTTP 端口，也初始化一个默认的 Metrics 实例用于内部采集
		metricsInstance = metrics.NewMetrics(b.serviceName)
	}

	// 3.1 初始化限流 (Rate Limiting)：如果启用，则在 Gin 中间件链中添加限流器。
	// 使用本地令牌桶限流，Rate 为每秒允许的请求数，Burst 为突发容量。
	if cfg.RateLimit.Enabled {
		rate := cfg.RateLimit.Rate
		burst := cfg.RateLimit.Burst
		if rate <= 0 {
			rate = 1000 // 默认每秒 1000 请求
		}
		if burst <= 0 {
			burst = 100 // 默认突发容量 100
		}
		logger.Logger.Info("Rate limit middleware enabled", "rate", rate, "burst", burst)
		b.ginMiddleware = append(b.ginMiddleware, middleware.NewLocalRateLimitMiddleware(rate, burst))
	}

	// 3.2 初始化熔断 (Circuit Breaker)：如果启用，则在 Gin 中间件链中添加熔断器。
	// 熔断器会在错误率过高时自动拒绝请求，防止雪崩效应。
	if cfg.CircuitBreaker.Enabled {
		logger.Logger.Info("Circuit breaker middleware enabled")
		b.ginMiddleware = append(b.ginMiddleware, middleware.HttpCircuitBreaker(cfg.CircuitBreaker, metricsInstance))
	}

	// 3.3 注入标准 Metrics 中间件
	b.ginMiddleware = append(b.ginMiddleware, middleware.HttpMetricsMiddleware(metricsInstance))
	b.grpcInterceptors = append(b.grpcInterceptors, middleware.GrpcMetricsInterceptor(metricsInstance))

	// 5. 依赖注入与核心业务初始化。
	// 调用用户注册的 initService 函数，创建具体的 Repository、Service 和 Facade。
	serviceInstance, cleanup, err := b.initService.(func(any, *metrics.Metrics) (any, func(), error))(b.configInstance, metricsInstance)
	if err != nil {
		logger.Logger.Error("failed to initialize service", "error", err)
		panic(err)
	}
	b.appOpts = append(b.appOpts, WithCleanup(cleanup))

	// 6. 协议层服务创建与注册。
	var servers []server.Server
	// 创建 gRPC 服务器并注册业务 Handler。
	if b.registerGRPC != nil {
		grpcAddr := fmt.Sprintf("%s:%d", cfg.Server.GRPC.Addr, cfg.Server.GRPC.Port)
		grpcSrv := server.NewGRPCServer(grpcAddr, logger.Logger, func(s *grpc.Server) {
			b.registerGRPC.(func(*grpc.Server, any))(s, serviceInstance)
		}, b.grpcInterceptors...)
		servers = append(servers, grpcSrv)
	}
	// 创建 Gin HTTP 服务器并注册路由。
	if b.registerGin != nil {
		httpAddr := fmt.Sprintf("%s:%d", cfg.Server.HTTP.Addr, cfg.Server.HTTP.Port)
		ginEngine := server.NewDefaultGinEngine(logger.Logger, b.ginMiddleware...)

		// --- 自动注册标准运维接口 (核心架构增强：减少样板代码) ---
		sys := ginEngine.Group("/sys")
		{
			sys.GET("/health", func(c *gin.Context) {
				response.SuccessWithRawData(c, gin.H{
					"status":    "UP",
					"service":   b.serviceName,
					"timestamp": time.Now().Unix(),
				})
			})
			sys.GET("/ready", func(c *gin.Context) {
				response.SuccessWithRawData(c, gin.H{"status": "READY"})
			})
		}

		// 自动暴露指标接口
		if cfg.Metrics.Enabled && metricsInstance != nil {
			path := cfg.Metrics.Path
			if path == "" {
				path = "/metrics"
			}
			ginEngine.GET(path, gin.WrapH(metricsInstance.Handler()))
			logger.Logger.Info("Metrics endpoint registered", "path", path)
		}
		// -------------------------------------------------------

		b.registerGin.(func(*gin.Engine, any))(ginEngine, serviceInstance)
		ginSrv := server.NewGinServer(ginEngine, httpAddr, logger.Logger)
		servers = append(servers, ginSrv)
	}
	// 将创建好的服务器实例添加到 App 的生命周期管理中。
	b.appOpts = append(b.appOpts, WithServer(servers...))

	// 7. 返回配置完成的应用实例。
	return New(b.serviceName, logger.Logger, b.appOpts...)
}
