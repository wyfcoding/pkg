// Package app 提供了应用程序生命周期管理的基础设施.
package app

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/middleware"
	"github.com/wyfcoding/pkg/response"
	"github.com/wyfcoding/pkg/server"
	"github.com/wyfcoding/pkg/tracing"
	"google.golang.org/grpc"
)

const (
	defaultMetricsPath = "/metrics"
)

// Builder 提供了构建 App 的灵活方式.
type Builder struct {
	serviceName      string
	configInstance   any
	initService      any
	registerGRPC     any
	registerGin      any
	metricsPort      string
	appOpts          []Option
	healthCheckers   []func() error
	grpcInterceptors []grpc.UnaryServerInterceptor
	ginMiddleware    []gin.HandlerFunc
}

// NewBuilder 创建一个新的应用构建器.
func NewBuilder(serviceName string) *Builder {
	return &Builder{
		serviceName:      serviceName,
		appOpts:          make([]Option, 0),
		healthCheckers:   make([]func() error, 0),
		grpcInterceptors: make([]grpc.UnaryServerInterceptor, 0),
		ginMiddleware:    make([]gin.HandlerFunc, 0),
		configInstance:   nil,
		initService:      nil,
		registerGRPC:     nil,
		registerGin:      nil,
		metricsPort:      "",
	}
}

// WithConfig 设置配置实例.
func (b *Builder) WithConfig(conf any) *Builder {
	b.configInstance = conf

	return b
}

// WithGRPC 注册 gRPC 服务注册钩子.
func (b *Builder) WithGRPC(register func(*grpc.Server, any)) *Builder {
	b.registerGRPC = register

	return b
}

// WithGin 注册 Gin 路由注册钩子.
func (b *Builder) WithGin(register func(*gin.Engine, any)) *Builder {
	b.registerGin = register

	return b
}

// WithService 注册核心业务初始化逻辑.
func (b *Builder) WithService(init func(any, *metrics.Metrics) (any, func(), error)) *Builder {
	b.initService = init

	return b
}

// WithMetrics 在指定端口启用指标暴露.
func (b *Builder) WithMetrics(port string) *Builder {
	b.metricsPort = port

	return b
}

// WithHealthChecker 添加自定义健康检查.
func (b *Builder) WithHealthChecker(checker func() error) *Builder {
	b.healthCheckers = append(b.healthCheckers, checker)

	return b
}

// WithGRPCInterceptor 添加 gRPC 拦截器.
func (b *Builder) WithGRPCInterceptor(interceptors ...grpc.UnaryServerInterceptor) *Builder {
	b.grpcInterceptors = append(b.grpcInterceptors, interceptors...)

	return b
}

// WithGinMiddleware 添加 Gin 中间件.
func (b *Builder) WithGinMiddleware(mw ...gin.HandlerFunc) *Builder {
	b.ginMiddleware = append(b.ginMiddleware, mw...)

	return b
}

// Build 构建并组装完整的 App 实例.
func (b *Builder) Build() *App {
	cfg := b.loadConfig()
	loggerInstance := b.initLogger(&cfg)

	if cfg.Tracing.Enabled {
		b.initTracing(&cfg, loggerInstance)
	}

	metricsInstance := b.initMetrics(&cfg)

	b.setupMiddleware(&cfg, metricsInstance)

	serviceInstance, cleanup := b.assembleService(metricsInstance, loggerInstance)
	b.appOpts = append(b.appOpts, WithCleanup(cleanup))

	b.registerServers(&cfg, serviceInstance, metricsInstance, loggerInstance)

	for _, checker := range b.healthCheckers {
		b.appOpts = append(b.appOpts, WithHealthChecker(checker))
	}

	return New(b.serviceName, loggerInstance.Logger, b.appOpts...)
}

func (b *Builder) loadConfig() config.Config {
	configPath := fmt.Sprintf("./configs/%s/config.toml", b.serviceName)
	var flagPath string
	flag.StringVar(&flagPath, "conf", configPath, "path to config file")
	flag.Parse()

	if err := config.Load(flagPath, b.configInstance); err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	var cfg config.Config
	if c, ok := b.configInstance.(*config.Config); ok {
		cfg = *c
	} else {
		val := reflect.ValueOf(b.configInstance).Elem()
		found := false

		for i := range val.NumField() {
			field := val.Type().Field(i)
			if (field.Name == "Config" || field.Anonymous) && field.Type == reflect.TypeFor[config.Config]() {
				cfg = val.Field(i).Interface().(config.Config)
				found = true

				break
			}
		}

		if !found {
			panic("invalid config instance format")
		}
	}

	return cfg
}

func (b *Builder) initLogger(cfg *config.Config) *logging.Logger {
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

	if cfg.Log.Output != "file" {
		logConfig.File = ""
	}

	loggerInstance := logging.NewFromConfig(&logConfig)
	slog.SetDefault(loggerInstance.Logger)

	return loggerInstance
}

func (b *Builder) initTracing(cfg *config.Config, logger *logging.Logger) {
	shutdown, err := tracing.InitTracer(cfg.Tracing)
	if err != nil {
		logger.Logger.Error("failed to initialize tracer", "error", err)

		return
	}

	b.appOpts = append(b.appOpts, WithCleanup(func() {
		if cleanupErr := shutdown(context.Background()); cleanupErr != nil {
			logger.Logger.Error("failed to shutdown tracer", "error", cleanupErr)
		}
	}))

	b.ginMiddleware = append([]gin.HandlerFunc{middleware.TracingMiddleware(b.serviceName)}, b.ginMiddleware...)
}

func (b *Builder) initMetrics(cfg *config.Config) *metrics.Metrics {
	metricsPort := b.metricsPort
	if metricsPort == "" && cfg.Metrics.Enabled {
		metricsPort = cfg.Metrics.Port
	}

	metricsInstance := metrics.NewMetrics(b.serviceName)

	if metricsPort != "" {
		cleanup := metricsInstance.ExposeHTTP(metricsPort)
		b.appOpts = append(b.appOpts, WithCleanup(cleanup))
	}

	return metricsInstance
}

func (b *Builder) setupMiddleware(cfg *config.Config, m *metrics.Metrics) {
	if cfg.CircuitBreaker.Enabled {
		b.ginMiddleware = append(b.ginMiddleware, middleware.HTTPCircuitBreaker(cfg.CircuitBreaker, m))
	}

	b.ginMiddleware = append(b.ginMiddleware, middleware.HTTPMetricsMiddleware(m))
	b.grpcInterceptors = append(b.grpcInterceptors, middleware.GRPCMetricsInterceptor(m))
}

func (b *Builder) assembleService(m *metrics.Metrics, logger *logging.Logger) (instance any, cleanup func()) {
	initFn, ok := b.initService.(func(any, *metrics.Metrics) (any, func(), error))
	if !ok {
		panic("invalid initService format")
	}

	instance, cleanup, err := initFn(b.configInstance, m)
	if err != nil {
		logger.Logger.Error("failed to initialize service", "error", err)
		panic(err)
	}

	return instance, cleanup
}

func (b *Builder) registerServers(cfg *config.Config, svc any, m *metrics.Metrics, logger *logging.Logger) {
	var servers []server.Server

	if b.registerGRPC != nil {
		addr := fmt.Sprintf("%s:%d", cfg.Server.GRPC.Addr, cfg.Server.GRPC.Port)
		srv := server.NewGRPCServer(addr, logger.Logger, func(s *grpc.Server) {
			if reg, ok := b.registerGRPC.(func(*grpc.Server, any)); ok {
				reg(s, svc)
			}
		}, b.grpcInterceptors...)

		servers = append(servers, srv)
	}

	if b.registerGin != nil {
		addr := fmt.Sprintf("%s:%d", cfg.Server.HTTP.Addr, cfg.Server.HTTP.Port)
		engine := server.NewDefaultGinEngine(b.ginMiddleware...)

		b.registerAdminRoutes(engine, cfg, m)

		if reg, ok := b.registerGin.(func(*gin.Engine, any)); ok {
			reg(engine, svc)
		}

		servers = append(servers, server.NewGinServer(engine, addr, logger.Logger))
	}

	b.appOpts = append(b.appOpts, WithServer(servers...))
}

func (b *Builder) registerAdminRoutes(engine *gin.Engine, cfg *config.Config, m *metrics.Metrics) {
	sys := engine.Group("/sys")
	sys.GET("/health", func(c *gin.Context) {
		response.SuccessWithRawData(c, gin.H{
			"status":    "UP",
			"service":   b.serviceName,
			"timestamp": time.Now().Unix(),
		})
	})

	if cfg.Metrics.Enabled && m != nil {
		path := cfg.Metrics.Path
		if path == "" {
			path = defaultMetricsPath
		}

		engine.GET(path, gin.WrapH(m.Handler()))
	}
}
