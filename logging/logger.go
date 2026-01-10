// Package logging 提供了基于标准库 slog 深度定制的生产级日志系统。
// 核心增强特性：
// 1. 自动注入分布式追踪上下文（trace_id, span_id）。
// 2. 支持日志文件按天或按大小自动滚动切割（Lumberjack）。
// 3. 完美兼容 GORM v2 接口，实现 SQL 慢查询自动审计。
// 4. 全量结构化输出，符合 ELK/Prometheus 采集标准。
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/gorm/logger"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// LevelDebug 调试级别。
	LevelDebug = "debug"
	// LevelInfo 信息级别。
	LevelInfo = "info"
	// LevelWarn 警告级别。
	LevelWarn = "warn"
	// LevelError 错误级别。
	LevelError = "error"
)

var (
	// defaultLogger 是全局默认的 Logger 实例，采用单例模式。
	defaultLogger *Logger
	// once 用于确保 InitLogger 函数只被执行一次，保证 defaultLogger 的单例性。
	once sync.Once
)

// Config 定义日志配置结构。
type Config struct {
	Service    string
	Module     string
	Level      string
	File       string // 日志文件路径，为空则只输出到标准输出。
	MaxSize    int    // 每个日志文件最大尺寸 (MB)。
	MaxBackups int    // 保留旧日志文件的最大个数。
	MaxAge     int    // 保留旧日志文件的最大天数。
	Compress   bool   // 是否压缩旧日志。
}

// Logger 结构体封装了原生的 *slog.Logger，并添加了服务名和模块名，方便在日志中区分来源。
type Logger struct {
	*slog.Logger
	Service string // 所属微服务名称。
	Module  string // 所属业务模块名称。
}

// TraceHandler 是一个 Handler 中间件，负责从 Context 中提取链路追踪信息并扁平化到日志条目中。
type TraceHandler struct {
	slog.Handler
}

// Handle 实现日志条目的最终处理，自动补全链路追踪字段。
func (h *TraceHandler) Handle(ctx context.Context, record slog.Record) error {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		// 严格遵循规范字段命名：trace_id, span_id。
		record.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}

	return h.Handler.Handle(ctx, record)
}

// NewFromConfig 创建一个新的 Logger 实例。
// 支持通过 Config 结构体配置日志切割。
func NewFromConfig(cfg Config) *Logger {
	var logLevel slog.Level

	switch cfg.Level {
	case LevelDebug:
		logLevel = slog.LevelDebug
	case LevelInfo:
		logLevel = slog.LevelInfo
	case LevelWarn:
		logLevel = slog.LevelWarn
	case LevelError:
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	replaceAttr := func(_ []string, attr slog.Attr) slog.Attr {
		if attr.Key == slog.TimeKey {
			attr.Key = "timestamp"
		}

		return attr
	}

	var handler slog.Handler

	if cfg.File != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}
		handler = slog.NewJSONHandler(fileWriter, &slog.HandlerOptions{
			Level:       logLevel,
			ReplaceAttr: replaceAttr,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:       logLevel,
			ReplaceAttr: replaceAttr,
		})
	}

	traceHandler := &TraceHandler{Handler: handler}

	loggerInstance := slog.New(traceHandler).With(
		slog.String("service", cfg.Service),
		slog.String("module", cfg.Module),
	)

	return &Logger{
		Logger:  loggerInstance,
		Service: cfg.Service,
		Module:  cfg.Module,
	}
}

// NewLogger 是创建一个带有简单参数的 logger 的兼容别名。
func NewLogger(service, module string, level ...string) *Logger {
	levelStr := LevelInfo
	if len(level) > 0 {
		levelStr = level[0]
	}

	return NewFromConfig(Config{
		Service: service,
		Module:  module,
		Level:   levelStr,
	})
}

// InitLogger 初始化全局默认日志记录器。
func InitLogger(service, module string, level ...string) {
	once.Do(func() {
		levelStr := LevelInfo
		if len(level) > 0 {
			levelStr = level[0]
		}

		defaultLogger = NewFromConfig(Config{
			Service: service,
			Module:  module,
			Level:   levelStr,
		})

		slog.SetDefault(defaultLogger.Logger)
	})
}

// EnsureDefaultLogger 确保默认日志记录器已初始化。
func EnsureDefaultLogger() {
	if defaultLogger == nil {
		InitLogger("default", "default", LevelInfo)
	}
}

// Default 返回默认日志记录器实例。
func Default() *Logger {
	EnsureDefaultLogger()

	return defaultLogger
}

// Info 记录 Info 级别日志。
func Info(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.InfoContext(ctx, msg, args...)
}

// Warn 记录 Warn 级别日志。
func Warn(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.WarnContext(ctx, msg, args...)
}

// Error 记录 Error 级别日志。
func Error(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.ErrorContext(ctx, msg, args...)
}

// Debug 记录 Debug 级别日志。
func Debug(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.DebugContext(ctx, msg, args...)
}

// InfoContext 兼容标准库接口。
func InfoContext(ctx context.Context, msg string, args ...any) {
	Info(ctx, msg, args...)
}

// WarnContext 兼容标准库接口。
func WarnContext(ctx context.Context, msg string, args ...any) {
	Warn(ctx, msg, args...)
}

// ErrorContext 兼容标准库接口。
func ErrorContext(ctx context.Context, msg string, args ...any) {
	Error(ctx, msg, args...)
}

// DebugContext 兼容标准库接口。
func DebugContext(ctx context.Context, msg string, args ...any) {
	Debug(ctx, msg, args...)
}

// LogDuration 记录操作耗时。
func LogDuration(ctx context.Context, operation string, args ...any) func() {
	start := time.Now()

	return func() {
		logArgs := append(args, "duration", time.Since(start))
		Info(ctx, fmt.Sprintf("%s finished", operation), logArgs...)
	}
}

// GetLogger 返回全局默认的 Logger 实例。
func GetLogger() *Logger {
	if defaultLogger == nil {
		return NewFromConfig(Config{Service: "unknown", Module: "unknown"})
	}

	return defaultLogger
}

// GormLogger 是一个自定义的 GORM 日志器。
type GormLogger struct {
	loggerInstance *slog.Logger
	SlowThreshold  time.Duration
}

// NewGormLogger 创建一个新的 GormLogger 实例。
func NewGormLogger(loggerObj *Logger, slowThreshold time.Duration) *GormLogger {
	return &GormLogger{
		loggerInstance: loggerObj.Logger,
		SlowThreshold:  slowThreshold,
	}
}

// LogMode 实现了 gorm logger.Interface 的 LogMode 方法。
func (l *GormLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l
}

// Info 实现了 gorm logger.Interface 的 Info 方法。
func (l *GormLogger) Info(ctx context.Context, msg string, data ...any) {
	l.loggerInstance.InfoContext(ctx, fmt.Sprintf(msg, data...))
}

// Warn 实现了 gorm logger.Interface 的 Warn 方法。
func (l *GormLogger) Warn(ctx context.Context, msg string, data ...any) {
	l.loggerInstance.WarnContext(ctx, fmt.Sprintf(msg, data...))
}

// Error 实现了 gorm logger.Interface 的 Error 方法。
func (l *GormLogger) Error(ctx context.Context, msg string, data ...any) {
	l.loggerInstance.ErrorContext(ctx, fmt.Sprintf(msg, data...))
}

// Trace 实现了 gorm logger.Interface 的 Trace 方法。
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()

	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(
			attribute.String("db.statement", sql),
			attribute.Int64("db.rows_affected", rows),
		)
	}

	fields := []any{
		slog.String("sql", sql),
		slog.Duration("elapsed", elapsed),
	}

	if rows != -1 {
		fields = append(fields, slog.Int64("rows", rows))
	}

	if err != nil && err != logger.ErrRecordNotFound {
		fields = append(fields, slog.Any("error", err))
		l.loggerInstance.ErrorContext(ctx, "gorm trace error", fields...)
	} else if l.SlowThreshold != 0 && elapsed > l.SlowThreshold {
		fields = append(fields, slog.String("type", "slow_query"))
		l.loggerInstance.WarnContext(ctx, "gorm trace slow query", fields...)
	} else {
		l.loggerInstance.DebugContext(ctx, "gorm trace", fields...)
	}
}