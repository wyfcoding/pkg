package app

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/middleware"
	"github.com/wyfcoding/pkg/server"
	"github.com/wyfcoding/pkg/tracing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

// Builder 提供了构建 App 的灵活方式。
type Builder struct {
	serviceName    string      // 服务名称
	configInstance interface{} // 配置实例
	appOpts        []Option    // 应用程序选项

	initService  interface{} // 服务初始化函数
	registerGRPC interface{} // gRPC 注册函数
	registerGin  interface{} // Gin 注册函数

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
func (b *Builder) WithConfig(conf interface{}) *Builder {
	b.configInstance = conf
	return b
}

// WithGRPC 注册 gRPC 服务器的创建逻辑。
// register 函数负责将具体的gRPC服务注册到 `*grpc.Server` 实例中。
func (b *Builder) WithGRPC(register func(*grpc.Server, interface{})) *Builder {
	b.registerGRPC = register
	return b
}

// WithGin 注册 Gin 服务器的创建逻辑。
// register 函数负责将HTTP路由注册到 `*gin.Engine` 实例中。
func (b *Builder) WithGin(register func(*gin.Engine, interface{})) *Builder {
	b.registerGin = register
	return b
}

// WithService 注册服务的初始化逻辑。
// init 函数接收配置和Metrics实例，并返回服务实例、清理函数和错误。
func (b *Builder) WithService(init func(interface{}, *metrics.Metrics) (interface{}, func(), error)) *Builder {
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
	// 1. 加载配置。
	configPath := fmt.Sprintf("./configs/%s/config.toml", b.serviceName)
	var flagConfigPath string
	flag.StringVar(&flagConfigPath, "conf", configPath, "配置文件路径")
	flag.Parse()

	if err := config.Load(flagConfigPath, b.configInstance); err != nil {
		panic(fmt.Sprintf("加载配置失败: %v", err))
	}

	// 提取 config.Config
	var cfg config.Config
	// 尝试直接断言
	if c, ok := b.configInstance.(*config.Config); ok {
		cfg = *c
	} else {
		// 使用反射从配置实例中获取 `config.Config` 结构。
		val := reflect.ValueOf(b.configInstance).Elem()
		// 检查是否是结构体
		if val.Kind() != reflect.Struct {
			panic("配置实例必须是指向结构体的指针")
		}
		// 查找名为 "Config" 的字段
		cfgField := val.FieldByName("Config")
		if cfgField.IsValid() && cfgField.Type() == reflect.TypeOf(config.Config{}) {
			cfg = cfgField.Interface().(config.Config)
		} else {
			// 如果找不到名为 Config 的字段，或者类型不对，尝试查找是否嵌入了 config.Config
			// 简单的查找嵌入字段（匿名主要字段）
			found := false
			for i := 0; i < val.NumField(); i++ {
				field := val.Type().Field(i)
				if field.Anonymous && field.Type == reflect.TypeOf(config.Config{}) {
					cfg = val.Field(i).Interface().(config.Config)
					found = true
					break
				}
			}
			if !found {
				// 最后尝试强制转换整个结构体（如果它就是 config.Config 的别名，虽然后者不太可能）
				panic("配置实例必须是 *config.Config，或者包含一个名为 'Config' 的 config.Config 类型字段/嵌入字段")
			}
		}
	}

	// 2. 初始化日志。
	// 将配置中的 LogConfig 映射到 logging.Config
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
	// 如果配置指定 Output 为文件但没有提供路径，或者 Output 不是 "file"，则清空 File 字段以确保输出到 stdout
	if cfg.Log.Output != "file" {
		logConfig.File = ""
	}

	logger := logging.NewFromConfig(logConfig)
	// 设置为全局默认 Logger，使得 slog.Info 等可以直接使用配置好的 Logger
	slog.SetDefault(logger.Logger)

	// 3. 初始化分布式追踪 (OpenTelemetry)
	if cfg.Tracing.Enabled {
		shutdown, err := tracing.InitTracer(cfg.Tracing)
		if err != nil {
			logger.Logger.Error("Failed to initialize tracer", "error", err)
		} else {
			b.appOpts = append(b.appOpts, WithCleanup(func() {
				if err := shutdown(context.Background()); err != nil {
					logger.Logger.Error("Failed to shutdown tracer", "error", err)
				}
			}))
			logger.Logger.Info("Tracer initialized", "endpoint", cfg.Tracing.OTLPEndpoint)
			
			// 自动添加 Gin 追踪中间件
			// 注意：这会将追踪中间件放在最前面，确保覆盖整个请求生命周期
			b.ginMiddleware = append([]gin.HandlerFunc{middleware.TracingMiddleware(b.serviceName)}, b.ginMiddleware...)
		}
	}

	// 4. (可选) 初始化 Metrics。
	var metricsInstance *metrics.Metrics
	// 如果未通过 WithMetrics 指定端口，尝试从配置中读取
	metricsPort := b.metricsPort
	if metricsPort == "" && cfg.Metrics.Enabled {
		metricsPort = cfg.Metrics.Port
	}

	if metricsPort != "" {
		metricsInstance = metrics.NewMetrics(b.serviceName)
		metricsCleanup := metricsInstance.ExposeHttp(metricsPort)
		b.appOpts = append(b.appOpts, WithCleanup(metricsCleanup))
	}

	// 5. 依赖注入：初始化核心服务。
	// 使用类型断言调用注册的服务初始化函数。
	serviceInstance, cleanup, err := b.initService.(func(interface{}, *metrics.Metrics) (interface{}, func(), error))(b.configInstance, metricsInstance)
	if err != nil {
		logger.Logger.Error("初始化服务失败", "error", err)
		panic(err)
	}
	b.appOpts = append(b.appOpts, WithCleanup(cleanup))

	// 6. 创建服务器。
	var servers []server.Server
	// 如果注册了gRPC服务，则创建gRPC服务器。
	if b.registerGRPC != nil {
		grpcAddr := fmt.Sprintf("%s:%d", cfg.Server.GRPC.Addr, cfg.Server.GRPC.Port)
		grpcSrv := server.NewGRPCServer(grpcAddr, logger.Logger, func(s *grpc.Server) {
			b.registerGRPC.(func(*grpc.Server, interface{}))(s, serviceInstance)
		}, b.grpcInterceptors...)
		servers = append(servers, grpcSrv)
	}
	// 如果注册了Gin服务，则创建Gin HTTP服务器。
	if b.registerGin != nil {
		httpAddr := fmt.Sprintf("%s:%d", cfg.Server.HTTP.Addr, cfg.Server.HTTP.Port)
		ginEngine := server.NewDefaultGinEngine(logger.Logger, b.ginMiddleware...)
		b.registerGin.(func(*gin.Engine, interface{}))(ginEngine, serviceInstance)
		ginSrv := server.NewGinServer(ginEngine, httpAddr, logger.Logger)
		servers = append(servers, ginSrv)
	}
	b.appOpts = append(b.appOpts, WithServer(servers...))

	// 7. 创建应用实例。
	return New(b.serviceName, logger.Logger, b.appOpts...)
}