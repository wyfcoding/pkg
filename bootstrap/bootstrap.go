package bootstrap

import (
	"context"
	"flag"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/tracing"
)

// Config 包含用于启动服务的通用配置
type Config struct {
	ServiceName string
	Version     string
	ConfigPath  string
}

// Bootstrapper 处理通用基础设施的初始化
type Bootstrapper struct {
	ServiceName string
	Version     string
	Logger      *logging.Logger
}

// New creates a new Bootstrapper
func New(serviceName, version string) *Bootstrapper {
	return &Bootstrapper{
		ServiceName: serviceName,
		Version:     version,
	}
}

// Initialize 解析标志、加载配置并设置日志和追踪
// 它返回加载的配置对象（应该是指向结构体的指针）
func (b *Bootstrapper) Initialize(cfg interface{}) error {
	var configPath string
	flag.StringVar(&configPath, "config", "configs/config.toml", "path to config file")
	flag.Parse()

	// 1. 临时初始化 Logger（用于记录配置加载错误）
	logging.InitLogger(b.ServiceName, "bootstrap")
	b.Logger = logging.Default()

	// 2. Load Configuration
	if err := config.Load(configPath, cfg); err != nil {
		b.Logger.Error("failed to load config", "error", err)
		return err
	}

	// 3. 使用配置重新初始化 Logger（如果配置包含日志设置）
	// 目前我们假设使用标准设置，但在实际场景中，我们会读取 cfg.Log
	// Since cfg is an interface{}, we might need reflection or a specific interface to extract LogConfig
	// 为了保持简单和通用，我们依赖初始 logger 或假设用户会在需要时重新初始化。
	// 但是，如果我们假设配置中存在标准的 TracingConfig 结构，我们可以初始化追踪。

	return nil
}

// SetupTracing 初始化 OpenTelemetry 追踪器
func (b *Bootstrapper) SetupTracing(cfg config.TracingConfig) func() {
	shutdown, err := tracing.InitTracer(cfg)
	if err != nil {
		b.Logger.Error("failed to init tracer", "error", err)
		return func() {}
	}
	return func() {
		if err := shutdown(context.Background()); err != nil {
			b.Logger.Error("failed to shutdown tracer", "error", err)
		}
	}
}
