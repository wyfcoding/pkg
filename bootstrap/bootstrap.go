package bootstrap

import (
	"context"
	"flag"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/tracing"
)

// Config holds the common configuration for bootstrapping a service
type Config struct {
	ServiceName string
	Version     string
	ConfigPath  string
}

// Bootstrapper handles the initialization of common infrastructure
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

// Initialize parses flags, loads config, and sets up logging and tracing
// It returns the loaded configuration object (which should be a pointer to a struct)
func (b *Bootstrapper) Initialize(cfg interface{}) error {
	var configPath string
	flag.StringVar(&configPath, "config", "configs/config.toml", "path to config file")
	flag.Parse()

	// 1. Initialize Logger temporarily (to log config loading errors)
	logging.InitLogger(b.ServiceName, "bootstrap")
	b.Logger = logging.Default()

	// 2. Load Configuration
	if err := config.Load(configPath, cfg); err != nil {
		b.Logger.Error("failed to load config", "error", err)
		return err
	}

	// 3. Re-initialize Logger with config (if config has log settings)
	// For now, we assume standard settings, but in a real scenario, we'd read cfg.Log
	// Since cfg is an interface{}, we might need reflection or a specific interface to extract LogConfig
	// To keep it simple and generic, we rely on the initial logger or assume the user will re-init if needed.
	// However, we CAN init tracing here if we assume a standard TracingConfig structure in the config.

	return nil
}

// SetupTracing initializes the OpenTelemetry tracer
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
