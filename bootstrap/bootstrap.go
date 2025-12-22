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

// New 创建一个新的引导器实例
func New(serviceName, version string) *Bootstrapper {
	return &Bootstrapper{
		ServiceName: serviceName,
		Version:     version,
	}
}

// Initialize 解析命令行标志、加载配置文件，并初步初始化日志系统。
// 它接收一个 cfg 指针，用于将加载的配置反序列化到该结构体中。
func (b *Bootstrapper) Initialize(cfg any) error {
	var configPath string
	flag.StringVar(&configPath, "config", "configs/config.toml", "path to config file")
	flag.Parse()

	// 1. 临时初始化 Logger（用于记录配置加载过程中的潜在错误）。
	logging.InitLogger(b.ServiceName, "bootstrap")
	b.Logger = logging.Default()

	// 2. 加载配置文件：读取 TOML 文件并映射到传入的 cfg 结构体中。
	if err := config.Load(configPath, cfg); err != nil {
		b.Logger.Error("failed to load config", "error", err)
		return err
	}

	// 3. 使用配置重新初始化 Logger（如果配置包含特定的日志设置）。
	// 目前我们假设使用标准设置，但在复杂场景下，可以根据 cfg.Log 的内容进行二次初始化。
	// 由于 cfg 是 interface{} 类型，可能需要使用反射或特定的接口来提取 LogConfig。
	// 为了保持引导器的简单和通用性，我们目前依赖默认初始化的 logger。

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
