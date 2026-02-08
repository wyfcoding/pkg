// Package app 提供了应用程序生命周期管理的基础设施.
// 生成摘要:
// 1) 默认接入 Trace ID 响应头、上下文增强、HTTP 错误处理、gRPC Request ID、访问日志与错误翻译拦截器，统一链路字段输出与错误码映射。
// 2) 支持按配置启用 HTTP/gRPC 超时、限流与并发保护。
// 3) 调整 gRPC 拦截器顺序，优先确保 Panic 可被统一恢复。
// 假设:
// 1) 业务侧沿用 builder 默认拦截器顺序即可满足观测需求。
package app

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"time"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/health"
	"github.com/wyfcoding/pkg/httpclient"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/middleware"
	"github.com/wyfcoding/pkg/security"
	"github.com/wyfcoding/pkg/server"
	"github.com/wyfcoding/pkg/tracing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

const (
	defaultMetricsPath = "/metrics"
)

// Initializer 定义了业务服务的初始化函数原型。
type Initializer[C any, S any] func(cfg C, m *metrics.Metrics) (S, func(), error)

// GRPCRegistrar 定义了 gRPC 服务注册函数原型。
type GRPCRegistrar[S any] func(s *grpc.Server, svc S)

// GinRegistrar 定义了 Gin 路由注册函数原型。
type GinRegistrar[S any] func(e *gin.Engine, svc S)

// Builder 提供了构建 App 的灵活方式，通过泛型支持强类型的配置与服务实例。
type Builder[C any, S any] struct {
	serviceName      string
	configInstance   C
	initService      Initializer[C, S]
	registerGRPC     GRPCRegistrar[S]
	registerGin      GinRegistrar[S]
	wsManager        *server.WSManager
	metricsPort      string
	appOpts          []Option
	healthCheckers   []func() error
	grpcInterceptors []grpc.UnaryServerInterceptor
	ginMiddleware    []gin.HandlerFunc
}

// NewBuilder 创建一个新的应用构建器。
func NewBuilder[C any, S any](serviceName string) *Builder[C, S] {
	return &Builder[C, S]{
		serviceName:      serviceName,
		appOpts:          make([]Option, 0),
		healthCheckers:   make([]func() error, 0),
		grpcInterceptors: make([]grpc.UnaryServerInterceptor, 0),
		ginMiddleware:    make([]gin.HandlerFunc, 0),
	}
}

// WithConfig 设置配置实例。
func (b *Builder[C, S]) WithConfig(conf C) *Builder[C, S] {
	b.configInstance = conf

	return b
}

// WithGRPC 注册 gRPC 服务注册钩子。
func (b *Builder[C, S]) WithGRPC(register GRPCRegistrar[S]) *Builder[C, S] {
	b.registerGRPC = register

	return b
}

// WithGin 注册 Gin 路由注册钩子。
func (b *Builder[C, S]) WithGin(register GinRegistrar[S]) *Builder[C, S] {
	b.registerGin = register

	return b
}

// WithService 注册核心业务初始化逻辑。
func (b *Builder[C, S]) WithService(init Initializer[C, S]) *Builder[C, S] {
	b.initService = init

	return b
}

// WithMetrics 在指定端口启用指标暴露。
func (b *Builder[C, S]) WithMetrics(port string) *Builder[C, S] {
	b.metricsPort = port

	return b
}

// WithHealthChecker 添加自定义健康检查。
func (b *Builder[C, S]) WithHealthChecker(checker func() error) *Builder[C, S] {
	b.healthCheckers = append(b.healthCheckers, checker)

	return b
}

// WithGRPCInterceptor 添加 gRPC 拦截器。
func (b *Builder[C, S]) WithGRPCInterceptor(interceptors ...grpc.UnaryServerInterceptor) *Builder[C, S] {
	b.grpcInterceptors = append(b.grpcInterceptors, interceptors...)

	return b
}

// WithGinMiddleware 添加 Gin 中间件。
func (b *Builder[C, S]) WithGinMiddleware(mw ...gin.HandlerFunc) *Builder[C, S] {
	b.ginMiddleware = append(b.ginMiddleware, mw...)

	return b
}

// WithWebSocket 启用 WebSocket 支持.
func (b *Builder[C, S]) WithWebSocket(path string) *Builder[C, S] {
	b.wsManager = server.NewWSManager(logging.Default().Logger)
	b.WithGin(func(e *gin.Engine, _ S) {
		e.GET(path, func(c *gin.Context) {
			b.wsManager.ServeHTTP(c.Writer, c.Request)
		})
	})

	return b
}

// Build 构建并组装完整的 App 实例。
func (b *Builder[C, S]) Build() *App {
	cfg := b.loadConfig()
	loggerInstance := b.initLogger(&cfg)

	// Removed pprof auto-start to avoid G108 (Profiling endpoint exposed).
	// If pprof is needed, please register it manually on a secure port or internal network.

	if b.wsManager != nil {
		b.appOpts = append(b.appOpts, WithCleanup(func() {
			// WebSocket manager cleanup can be added here
		}))
		// 启动 WS Manager
		go b.wsManager.Run(context.Background())
	}

	if cfg.Tracing.Enabled {
		b.initTracing(&cfg, loggerInstance)
	}

	metricsInstance := b.initMetrics(&cfg)
	if metricsInstance != nil {
		metricsInstance.RegisterBuildInfo(b.serviceName, cfg.Version)
	}
	httpclient.SetDefault(httpclient.NewFromConfig(cfg.HTTPClient, loggerInstance, metricsInstance))

	b.setupMiddleware(&cfg, metricsInstance)

	serviceInstance, cleanup := b.assembleService(metricsInstance, loggerInstance)
	b.appOpts = append(b.appOpts, WithCleanup(cleanup))

	b.registerServers(&cfg, serviceInstance, metricsInstance, loggerInstance)

	for _, checker := range b.healthCheckers {
		b.appOpts = append(b.appOpts, WithHealthChecker(checker))
	}

	return New(b.serviceName, loggerInstance.Logger, b.appOpts...)
}

func (b *Builder[C, S]) loadConfig() config.Config {
	configPath := fmt.Sprintf("./configs/%s/config.toml", b.serviceName)
	var flagPath string
	flag.StringVar(&flagPath, "conf", configPath, "path to config file")
	flag.Parse()

	if err := config.Load(flagPath, b.configInstance); err != nil {
		panic("failed to load config: " + err.Error())
	}

	var cfg config.Config
	switch c := any(b.configInstance).(type) {
	case *config.Config:
		cfg = *c
	case config.Config:
		cfg = c
	default:
		// 使用反射尝试提取嵌套的 config.Config
		val := reflect.ValueOf(b.configInstance)
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}

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
			panic("invalid config instance format: must be *config.Config or embed config.Config")
		}
	}

	return cfg
}

func (b *Builder[C, S]) initLogger(cfg *config.Config) *logging.Logger {
	logConfig := logging.Config{
		Service:    b.serviceName,
		Module:     "app",
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
		Output:     cfg.Log.Output,
		File:       cfg.Log.File,
		MaxSize:    cfg.Log.MaxSize,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAge:     cfg.Log.MaxAge,
		Compress:   cfg.Log.Compress,
		Remote: logging.RemoteConfig{
			Enabled:       cfg.Log.Remote.Enabled,
			Endpoint:      cfg.Log.Remote.Endpoint,
			AuthToken:     cfg.Log.Remote.AuthToken,
			Timeout:       cfg.Log.Remote.Timeout,
			BatchSize:     cfg.Log.Remote.BatchSize,
			BufferSize:    cfg.Log.Remote.BufferSize,
			FlushInterval: cfg.Log.Remote.FlushInterval,
			DropOnFull:    cfg.Log.Remote.DropOnFull,
		},
	}

	if cfg.Log.Output != "file" {
		logConfig.File = ""
	}

	loggerInstance := logging.NewFromConfig(&logConfig)
	slog.SetDefault(loggerInstance.Logger)

	if loggerInstance != nil {
		b.appOpts = append(b.appOpts, WithCleanup(func() {
			if err := loggerInstance.Close(); err != nil {
				loggerInstance.Logger.Error("failed to close logger", "error", err)
			}
		}))
	}

	// 注册上下文提取器，将 contextx 中的业务字段自动注入日志
	logging.RegisterContextExtractor(func(ctx context.Context) []slog.Attr {
		var attrs []slog.Attr
		for _, key := range contextx.AllKeys {
			if val := ctx.Value(key); val != nil {
				if str, ok := val.(string); ok && str != "" {
					attrs = append(attrs, slog.String(contextx.KeyNames[key], str))
				}
			}
		}

		return attrs
	})

	return loggerInstance
}

func (b *Builder[C, S]) initTracing(cfg *config.Config, logger *logging.Logger) {
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
}

func (b *Builder[C, S]) initMetrics(cfg *config.Config) *metrics.Metrics {
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

func (b *Builder[C, S]) setupMiddleware(cfg *config.Config, m *metrics.Metrics) {
	// 添加基础中间件 (顺序很重要)
	baseGin := make([]gin.HandlerFunc, 0, 8)
	baseGin = append(baseGin, middleware.RequestID())
	if cfg.Tracing.Enabled {
		baseGin = append(baseGin, middleware.TracingMiddleware(b.serviceName))
	}
	baseGin = append(baseGin, middleware.TraceIDHeader())
	if cfg.CORS.Enabled {
		baseGin = append(baseGin, middleware.CORSWithOptions(middleware.CORSOptions{
			AllowOrigins:     cfg.CORS.AllowOrigins,
			AllowMethods:     cfg.CORS.AllowMethods,
			AllowHeaders:     cfg.CORS.AllowHeaders,
			ExposeHeaders:    cfg.CORS.ExposeHeaders,
			AllowCredentials: cfg.CORS.AllowCredentials,
			MaxAge:           cfg.CORS.MaxAge,
		}))
	}
	if cfg.Security.Enabled {
		baseGin = append(baseGin, middleware.SecurityHeadersWithConfig(cfg.Security))
	}
	if len(cfg.Security.IPAllowlist) > 0 {
		baseGin = append(baseGin, security.IPAllowlistMiddleware(cfg.Security.IPAllowlist))
	}
	if cfg.Maintenance.Enabled {
		baseGin = append(baseGin, middleware.MaintenanceMiddleware(cfg.Maintenance))
	}
	baseGin = append(baseGin,
		middleware.RequestContextEnricher(),
	)
	if cfg.Server.HTTP.MaxBodyBytes > 0 {
		baseGin = append(baseGin, middleware.MaxBodyBytes(cfg.Server.HTTP.MaxBodyBytes))
	}
	if cfg.RateLimit.Enabled && cfg.RateLimit.Rate > 0 {
		burst := cfg.RateLimit.Burst
		if burst <= 0 {
			burst = cfg.RateLimit.Rate
		}
		baseGin = append(baseGin, middleware.NewLocalRateLimitMiddlewareWithMetrics(cfg.RateLimit.Rate, burst, m))
	}
	if cfg.Concurrency.HTTP.Enabled && cfg.Concurrency.HTTP.Max > 0 {
		baseGin = append(baseGin, middleware.NewConcurrencyLimitMiddleware(cfg.Concurrency.HTTP.Max, cfg.Concurrency.HTTP.WaitTimeout))
	}

	if cfg.Server.HTTP.Timeout > 0 {
		baseGin = append(baseGin, middleware.TimeoutMiddleware(cfg.Server.HTTP.Timeout))
	}

	baseGin = append(baseGin,
		middleware.Recovery(),
		middleware.RequestLoggerWithOptions(middleware.RequestLoggerOptions{
			SlowThreshold: cfg.Log.SlowThreshold,
			SkipPaths:     []string{"/sys/health", "/metrics"},
		}),
		middleware.HTTPMetricsMiddlewareWithOptions(m, middleware.MetricsOptions{
			SlowThreshold: cfg.Log.SlowThreshold,
			SkipPaths:     []string{"/sys/health", "/metrics"},
		}),
		middleware.HTTPRequestSizeMiddleware(m),
		middleware.HTTPResponseSizeMiddleware(m),
		middleware.HTTPErrorHandler(),
	)

	if cfg.CircuitBreaker.Enabled {
		baseGin = append(baseGin, middleware.HTTPCircuitBreaker(cfg.CircuitBreaker, m))
	}

	b.ginMiddleware = append(baseGin, b.ginMiddleware...)

	grpcChain := make([]grpc.UnaryServerInterceptor, 0, 8)
	if cfg.Server.GRPC.Timeout > 0 {
		grpcChain = append(grpcChain, middleware.GRPCTimeoutInterceptor(cfg.Server.GRPC.Timeout))
	}

	grpcChain = append(grpcChain,
		middleware.GRPCRecovery(),
		middleware.GRPCErrorTranslator(),
		middleware.GRPCRequestID(),
		middleware.GRPCContextEnricher(),
	)
	if cfg.RateLimit.Enabled && cfg.RateLimit.Rate > 0 {
		burst := cfg.RateLimit.Burst
		if burst <= 0 {
			burst = cfg.RateLimit.Rate
		}
		grpcChain = append(grpcChain, middleware.NewGRPCLocalRateLimitInterceptorWithMetrics(cfg.RateLimit.Rate, burst, m))
	}
	if cfg.Concurrency.GRPC.Enabled && cfg.Concurrency.GRPC.Max > 0 {
		grpcChain = append(grpcChain, middleware.NewGRPCConcurrencyLimitInterceptor(cfg.Concurrency.GRPC.Max, cfg.Concurrency.GRPC.WaitTimeout))
	}

	grpcChain = append(grpcChain, middleware.GRPCRequestLoggerWithOptions(middleware.GRPCRequestLoggerOptions{
		SlowThreshold: cfg.Log.GRPCSlowThreshold,
	}))

	if cfg.CircuitBreaker.Enabled {
		grpcChain = append(grpcChain, middleware.GRPCCircuitBreaker(cfg.CircuitBreaker, m))
	}

	b.grpcInterceptors = append(grpcChain, b.grpcInterceptors...)
	b.grpcInterceptors = append(b.grpcInterceptors, middleware.GRPCMetricsInterceptorWithOptions(m, middleware.MetricsOptions{
		SlowThreshold: cfg.Log.GRPCSlowThreshold,
	}))
}

func (b *Builder[C, S]) assembleService(m *metrics.Metrics, logger *logging.Logger) (instance S, cleanup func()) {
	if b.initService == nil {
		panic("initService is required")
	}

	instance, cleanup, err := b.initService(b.configInstance, m)
	if err != nil {
		logger.Logger.Error("failed to initialize service", "error", err)
		panic(err)
	}

	return instance, cleanup
}

func (b *Builder[C, S]) registerServers(cfg *config.Config, svc S, m *metrics.Metrics, logger *logging.Logger) {
	var servers []server.Server

	if b.registerGRPC != nil {
		addr := fmt.Sprintf("%s:%d", cfg.Server.GRPC.Addr, cfg.Server.GRPC.Port)
		var maxStreams uint32
		if cfg.Server.GRPC.MaxConcurrentStreams > 0 {
			maxStreams = uint32(cfg.Server.GRPC.MaxConcurrentStreams)
		}
		grpcOptions := server.Options{
			ShutdownTimeout:          server.DefaultShutdownTimeout,
			GRPCMaxRecvMsgSize:       cfg.Server.GRPC.MaxRecvMsgSize,
			GRPCMaxSendMsgSize:       cfg.Server.GRPC.MaxSendMsgSize,
			GRPCMaxConcurrentStreams: maxStreams,
			GRPCKeepalive:            cfg.Server.GRPC.Keepalive,
		}
		srv := server.NewGRPCServer(addr, logger.Logger, func(s *grpc.Server) {
			healthCheckers := make([]health.Checker, 0, len(b.healthCheckers))
			for _, checker := range b.healthCheckers {
				healthCheckers = append(healthCheckers, health.Checker(checker))
			}
			health.RegisterGRPCHealthServer(s, b.serviceName, healthCheckers)
			b.registerGRPC(s, svc)
		}, b.grpcInterceptors, grpcOptions)

		servers = append(servers, srv)
	}

	if b.registerGin != nil {
		addr := fmt.Sprintf("%s:%d", cfg.Server.HTTP.Addr, cfg.Server.HTTP.Port)
		switch cfg.Server.Environment {
		case "prod":
			gin.SetMode(gin.ReleaseMode)
		case "test":
			gin.SetMode(gin.TestMode)
		default:
			gin.SetMode(gin.DebugMode)
		}
		engine := server.NewDefaultGinEngine(b.ginMiddleware...)
		if len(cfg.Server.HTTP.TrustedProxies) > 0 {
			if err := engine.SetTrustedProxies(cfg.Server.HTTP.TrustedProxies); err != nil {
				logger.Logger.Warn("failed to set trusted proxies", "error", err)
			}
		}
		httpOptions := server.Options{
			ShutdownTimeout:   server.DefaultShutdownTimeout,
			ReadTimeout:       cfg.Server.HTTP.ReadTimeout,
			ReadHeaderTimeout: cfg.Server.HTTP.ReadHeaderTimeout,
			WriteTimeout:      cfg.Server.HTTP.WriteTimeout,
			IdleTimeout:       cfg.Server.HTTP.IdleTimeout,
			MaxHeaderBytes:    cfg.Server.HTTP.MaxHeaderBytes,
		}

		b.registerAdminRoutes(engine, cfg, m)

		b.registerGin(engine, svc)

		servers = append(servers, server.NewGinServer(engine, addr, logger.Logger, httpOptions))
	}

	b.appOpts = append(b.appOpts, WithServer(servers...))
}

func (b *Builder[C, S]) registerAdminRoutes(engine *gin.Engine, cfg *config.Config, m *metrics.Metrics) {
	sys := engine.Group("/sys")
	healthChecks := func() (string, []gin.H) {
		status := "UP"
		checks := make([]gin.H, 0, len(b.healthCheckers))
		for idx, checker := range b.healthCheckers {
			start := time.Now()
			err := checker()
			item := gin.H{
				"name":        fmt.Sprintf("checker_%d", idx),
				"duration_ms": time.Since(start).Milliseconds(),
				"status":      "UP",
				"checked_at":  time.Now().Unix(),
			}
			if err != nil {
				item["status"] = "DOWN"
				item["error"] = err.Error()
				status = "DOWN"
			}
			checks = append(checks, item)
		}
		return status, checks
	}

	sys.GET("/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "UP",
			"service":   b.serviceName,
			"timestamp": time.Now().Unix(),
		})
	})
	sys.GET("/health", func(c *gin.Context) {
		status, checks := healthChecks()

		code := http.StatusOK
		if status != "UP" {
			code = http.StatusServiceUnavailable
		}

		c.JSON(code, gin.H{
			"status":    status,
			"service":   b.serviceName,
			"timestamp": time.Now().Unix(),
			"checks":    checks,
		})
	})
	sys.GET("/ready", func(c *gin.Context) {
		status, checks := healthChecks()

		code := http.StatusOK
		if status != "UP" {
			code = http.StatusServiceUnavailable
		}

		c.JSON(code, gin.H{
			"status":    status,
			"service":   b.serviceName,
			"timestamp": time.Now().Unix(),
			"checks":    checks,
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
