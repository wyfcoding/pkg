// Package logging 提供了统一的结构化日志（slog）封装，支持OpenTelemetry追踪上下文注入和GORM日志集成。
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/gorm/logger" // GORM的日志接口

	"go.opentelemetry.io/otel/trace" // OpenTelemetry追踪
)

var (
	// defaultLogger 是全局默认的Logger实例，采用单例模式。
	defaultLogger *Logger
	// once 用于确保InitLogger函数只被执行一次，保证defaultLogger的单例性。
	once sync.Once
)

// Config 定义日志配置
type Config struct {
	Service    string
	Module     string
	Level      string
	File       string // 日志文件路径，为空则只输出到 stdout
	MaxSize    int    // 每个日志文件最大尺寸 (MB)
	MaxBackups int    // 保留旧日志文件的最大个数
	MaxAge     int    // 保留旧日志文件的最大天数
	Compress   bool   // 是否压缩旧日志
}

// Logger 结构体封装了原生的 `*slog.Logger`，并添加了服务名和模块名，方便在日志中区分来源。
type Logger struct {
	*slog.Logger
	Service string // 服务名称
	Module  string // 模块名称
}

// TraceHandler 是一个自定义的 `slog.Handler` 装饰器，用于从 `context.Context` 中提取并注入 `trace_id` 和 `span_id` 到日志记录中。
type TraceHandler struct {
	slog.Handler
}

// Handle 方法实现了 `slog.Handler` 接口，在处理日志记录之前，
// 会尝试从上下文获取OpenTelemetry的SpanContext，如果有效，则将trace_id和span_id添加到日志属性中。
func (h *TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() { // 检查SpanContext是否有效，即是否存在正在进行的追踪
		r.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()), // 注入追踪ID
			slog.String("span_id", spanCtx.SpanID().String()),   // 注入Span ID
		)
	}
	return h.Handler.Handle(ctx, r) // 调用被装饰的原始Handler继续处理日志
}

// NewFromConfig 创建一个新的Logger实例。
// 支持通过 Config 结构体配置日志切割。
func NewFromConfig(cfg Config) *Logger {
	var logLevel slog.Level
	switch cfg.Level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	replaceAttr := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			a.Key = "timestamp"
		}
		return a
	}

	var handler slog.Handler

	// 如果配置了文件路径，则使用 lumberjack 进行日志切割
	if cfg.File != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    cfg.MaxSize, // MB
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge, // days
			Compress:   cfg.Compress,
		}
		// JSONHandler 输出到文件
		handler = slog.NewJSONHandler(fileWriter, &slog.HandlerOptions{
			Level:       logLevel,
			ReplaceAttr: replaceAttr,
		})
	} else {
		// 默认输出到 Stdout
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:       logLevel,
			ReplaceAttr: replaceAttr,
		})
	}

	// 使用TraceHandler装饰
	traceHandler := &TraceHandler{Handler: handler}

	logger := slog.New(traceHandler).With(
		slog.String("service", cfg.Service),
		slog.String("module", cfg.Module),
	)

	return &Logger{
		Logger:  logger,
		Service: cfg.Service,
		Module:  cfg.Module,
	}
}

// NewLogger 是创建一个带有简单参数的 logger 的兼容别名。
func NewLogger(service, module string, level ...string) *Logger {
	lvl := "info"
	if len(level) > 0 {
		lvl = level[0]
	}
	return NewFromConfig(Config{
		Service: service,
		Module:  module,
		Level:   lvl,
	})
}

// InitLogger 初始化全局默认日志记录器
// 兼容旧的参数列表，但推荐使用新的 NewLogger(Config)
func InitLogger(service, module string, level ...string) {
	once.Do(func() {
		lvl := "info"
		if len(level) > 0 {
			lvl = level[0]
		}
		defaultLogger = NewFromConfig(Config{
			Service: service,
			Module:  module,
			Level:   lvl,
		})
		slog.SetDefault(defaultLogger.Logger)
	})
}

// EnsureDefaultLogger 确保默认日志记录器已初始化
func EnsureDefaultLogger() {
	if defaultLogger == nil {
		InitLogger("default", "default", "info")
	}
}

// Default 返回默认日志记录器实例
func Default() *Logger {
	EnsureDefaultLogger()
	return defaultLogger
}

// Info 记录 Info 级别日志
func Info(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.InfoContext(ctx, msg, args...)
}

// Warn 记录 Warn 级别日志
func Warn(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.WarnContext(ctx, msg, args...)
}

// Error 记录 Error 级别日志
func Error(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.ErrorContext(ctx, msg, args...)
}

// Debug 记录 Debug 级别日志
func Debug(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.DebugContext(ctx, msg, args...)
}

// InfoContext 兼容接口
func InfoContext(ctx context.Context, msg string, args ...any) {
	Info(ctx, msg, args...)
}

// WarnContext 兼容接口
func WarnContext(ctx context.Context, msg string, args ...any) {
	Warn(ctx, msg, args...)
}

// ErrorContext 兼容接口
func ErrorContext(ctx context.Context, msg string, args ...any) {
	Error(ctx, msg, args...)
}

// DebugContext 兼容接口
func DebugContext(ctx context.Context, msg string, args ...any) {
	Debug(ctx, msg, args...)
}

// LogDuration 记录操作耗时
func LogDuration(ctx context.Context, operation string, args ...any) func() {
	start := time.Now()
	return func() {
		// 将耗时附加到日志参数中
		logArgs := append(args, "duration", time.Since(start))
		Info(ctx, fmt.Sprintf("%s finished", operation), logArgs...)
	}
}

// module: 日志所属的模块名称。
// 返回一个配置好的Logger实例，其日志输出格式为JSON，并默认包含服务名和模块名。

// InitLogger 初始化全局默认的Logger。
// 此函数应在应用程序启动时调用一次，以配置全局日志行为。

// GetLogger 返回全局默认的Logger实例。
// 如果尚未通过InitLogger初始化，它会返回一个带有"unknown"服务和模块的默认Logger。
func GetLogger() *Logger {
	if defaultLogger == nil {
		return NewFromConfig(Config{Service: "unknown", Module: "unknown"})
	}
	return defaultLogger
}

// GormLogger 是一个自定义的GORM日志器，它实现了 `gorm.io/gorm/logger.Interface` 接口，
// 从而允许GORM将数据库操作日志输出到统一的slog日志系统中。
type GormLogger struct {
	logger        *slog.Logger  // 用于输出日志的slog实例
	SlowThreshold time.Duration // 慢查询阈值，超过此阈值的SQL查询将被记录为警告
}

// NewGormLogger 创建一个新的GormLogger实例。
// logger: 使用的Logger实例。
// slowThreshold: 慢查询的持续时间阈值。
func NewGormLogger(logger *Logger, slowThreshold time.Duration) *GormLogger {
	return &GormLogger{
		logger:        logger.Logger,
		SlowThreshold: slowThreshold,
	}
}

// LogMode 实现了gorm logger.Interface的LogMode方法。
// 鉴于slog的设计，通常不直接在运行时动态更改现有logger实例的级别，
// 而是通过创建新的Handler或在HandlerOptions中配置。
// 此处直接返回自身，表示沿用当前logger的配置。
func (l *GormLogger) LogMode(level logger.LogLevel) logger.Interface {
	// GORM的LogLevel与slog的LogLevel映射关系需要自行定义，或者在此处根据level做过滤。
	// 目前简单返回自身，意味着GORM的日志级别控制将主要依赖于NewLogger中配置的slog级别。
	return l
}

// Info 实现了gorm logger.Interface的Info方法，将GORM的Info级别日志输出为slog的Info级别。
func (l *GormLogger) Info(ctx context.Context, msg string, data ...any) {
	l.logger.InfoContext(ctx, fmt.Sprintf(msg, data...))
}

// Warn 实现了gorm logger.Interface的Warn方法，将GORM的Warn级别日志输出为slog的Warn级别。
func (l *GormLogger) Warn(ctx context.Context, msg string, data ...any) {
	l.logger.WarnContext(ctx, fmt.Sprintf(msg, data...))
}

// Error 实现了gorm logger.Interface的Error方法，将GORM的Error级别日志输出为slog的Error级别。
func (l *GormLogger) Error(ctx context.Context, msg string, data ...any) {
	l.logger.ErrorContext(ctx, fmt.Sprintf(msg, data...))
}

// Trace 实现了gorm logger.Interface的Trace方法，用于记录SQL查询的详细信息，包括耗时、SQL语句和错误。
// 慢查询会以Warn级别记录，错误查询以Error级别记录，普通查询以Debug级别记录。
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin) // 计算SQL执行耗时
	sql, rows := fc()            // 获取SQL语句和影响的行数

	fields := []any{
		slog.String("sql", sql),           // SQL语句
		slog.Duration("elapsed", elapsed), // 执行耗时
	}
	if rows != -1 { // 检查是否返回了影响行数
		fields = append(fields, slog.Int64("rows", rows)) // 影响行数
	}

	if err != nil && err != logger.ErrRecordNotFound { // 如果存在错误且不是“未找到记录”错误
		fields = append(fields, slog.Any("error", err)) // 错误信息
		l.logger.ErrorContext(ctx, "gorm trace error", fields...)
	} else if l.SlowThreshold != 0 && elapsed > l.SlowThreshold { // 如果是慢查询
		fields = append(fields, slog.String("type", "slow_query"))
		l.logger.WarnContext(ctx, "gorm trace slow query", fields...)
	} else { // 普通查询
		l.logger.DebugContext(ctx, "gorm trace", fields...)
	}
}
