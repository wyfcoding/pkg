// Package logging 提供了基于标准库 slog 深度定制的生产级日志系统.
package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/gorm/logger"
)

const (
	// LevelDebug 调试级别.
	LevelDebug = "debug"
	// LevelInfo 信息级别.
	LevelInfo = "info"
	// LevelWarn 警告级别.
	LevelWarn = "warn"
	// LevelError 错误级别.
	LevelError = "error"
)

var (
	defaultLogger *Logger
	once          sync.Once
)

// Config 定义日志配置结构.
type Config struct {
	Service    string `json:"service"`     // 服务名称。
	Module     string `json:"module"`      // 模块名称。
	Level      string `json:"level"`       // 日志级别。
	File       string `json:"file"`        // 日志文件路径。
	MaxSize    int    `json:"max_size"`    // 单个文件最大大小 (MB)。
	MaxBackups int    `json:"max_backups"` // 最大备份数。
	MaxAge     int    `json:"max_age"`     // 最大保留天数。
	Compress   bool   `json:"compress"`    // 是否启用压缩。
}

// Logger 结构体封装了原生的 *slog.Logger.
type Logger struct {
	*slog.Logger
	Service string
	Module  string
}

// TraceHandler 是一个 Handler 中间件.
type TraceHandler struct {
	slog.Handler
}

// Handle 实现日志条目的最终处理.
// slog.Handler 接口要求 record 按值传递。
//
//nolint:gocritic
func (h *TraceHandler) Handle(ctx context.Context, record slog.Record) error {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}

	if err := h.Handler.Handle(ctx, record); err != nil {
		return fmt.Errorf("failed to handle log record: %w", err)
	}

	return nil
}

// NewFromConfig 创建一个新的 Logger 实例.
func NewFromConfig(cfg *Config) *Logger {
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

	opts := &slog.HandlerOptions{
		Level:       logLevel,
		ReplaceAttr: replaceAttr,
		AddSource:   false,
	}

	if cfg.File != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
			LocalTime:  true,
		}
		handler = slog.NewJSONHandler(fileWriter, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
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

// NewLogger 是创建一个带有简单参数的 logger 的兼容别名.
func NewLogger(service, module string, level ...string) *Logger {
	levelStr := LevelInfo
	if len(level) > 0 {
		levelStr = level[0]
	}

	return NewFromConfig(&Config{
		Service:    service,
		Module:     module,
		Level:      levelStr,
		File:       "",
		MaxSize:    0,
		MaxBackups: 0,
		MaxAge:     0,
		Compress:   false,
	})
}

// InitLogger 初始化全局默认日志记录器.
func InitLogger(service, module string, level ...string) {
	once.Do(func() {
		levelStr := LevelInfo
		if len(level) > 0 {
			levelStr = level[0]
		}

		defaultLogger = NewFromConfig(&Config{
			Service:    service,
			Module:     module,
			Level:      levelStr,
			File:       "",
			MaxSize:    0,
			MaxBackups: 0,
			MaxAge:     0,
			Compress:   false,
		})

		slog.SetDefault(defaultLogger.Logger)
	})
}

// EnsureDefaultLogger 确保默认日志记录器已初始化.
func EnsureDefaultLogger() {
	if defaultLogger == nil {
		InitLogger("default", "default", LevelInfo)
	}
}

// Default 返回默认日志记录器实例.
func Default() *Logger {
	EnsureDefaultLogger()

	return defaultLogger
}

// Info 记录 Info 级别日志.
func Info(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.InfoContext(ctx, msg, args...)
}

// Warn 记录 Warn 级别日志.
func Warn(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.WarnContext(ctx, msg, args...)
}

// Error 记录 Error 级别日志.
func Error(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.ErrorContext(ctx, msg, args...)
}

// Debug 记录 Debug 级别日志.
func Debug(ctx context.Context, msg string, args ...any) {
	EnsureDefaultLogger()
	defaultLogger.DebugContext(ctx, msg, args...)
}

// InfoContext 兼容标准库接口.
func InfoContext(ctx context.Context, msg string, args ...any) {
	Info(ctx, msg, args...)
}

// WarnContext 兼容标准库接口.
func WarnContext(ctx context.Context, msg string, args ...any) {
	Warn(ctx, msg, args...)
}

// ErrorContext 兼容标准库接口.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	Error(ctx, msg, args...)
}

// DebugContext 兼容标准库接口.
func DebugContext(ctx context.Context, msg string, args ...any) {
	Debug(ctx, msg, args...)
}

// LogDuration 记录操作耗时.
func LogDuration(ctx context.Context, operation string, args ...any) (done func()) {
	start := time.Now()

	return func() {
		logArgs := make([]any, 0, len(args)+2)
		logArgs = append(logArgs, args...)
		logArgs = append(logArgs, "duration", time.Since(start))
		Info(ctx, operation+" finished", logArgs...)
	}
}

// GetLogger 返回全局默认的 Logger 实例.
func GetLogger() *Logger {
	if defaultLogger == nil {
		return NewFromConfig(&Config{
			Service:    "unknown",
			Module:     "unknown",
			Level:      LevelInfo,
			File:       "",
			MaxSize:    0,
			MaxBackups: 0,
			MaxAge:     0,
			Compress:   false,
		})
	}

	return defaultLogger
}

// GormLogger 是一个自定义的 GORM 日志器.
type GormLogger struct {
	loggerInstance *slog.Logger
	SlowThreshold  time.Duration
}

// NewGormLogger 创建一个新的 GormLogger 实例.
func NewGormLogger(loggerObj *Logger, slowThreshold time.Duration) *GormLogger {
	return &GormLogger{
		loggerInstance: loggerObj.Logger,
		SlowThreshold:  slowThreshold,
	}
}

// LogMode 实现了 gorm logger.Interface 的 LogMode 方法.
func (l *GormLogger) LogMode(_ logger.LogLevel) logger.Interface {
	return l
}

// Info 实现了 gorm logger.Interface 的 Info 方法.
func (l *GormLogger) Info(ctx context.Context, msg string, data ...any) {
	l.loggerInstance.InfoContext(ctx, fmt.Sprintf(msg, data...))
}

// Warn 实现了 gorm logger.Interface 的 Warn 方法.
func (l *GormLogger) Warn(ctx context.Context, msg string, data ...any) {
	l.loggerInstance.WarnContext(ctx, fmt.Sprintf(msg, data...))
}

// Error 实现了 gorm logger.Interface 的 Error 方法.
func (l *GormLogger) Error(ctx context.Context, msg string, data ...any) {
	l.loggerInstance.ErrorContext(ctx, fmt.Sprintf(msg, data...))
}

// Trace 实现了 gorm logger.Interface 的 Trace 方法.
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

	fields := make([]any, 0, 4)
	fields = append(fields, slog.String("sql", sql), slog.Duration("elapsed", elapsed))

	if rows != -1 {
		fields = append(fields, slog.Int64("rows", rows))
	}

	switch {
	case err != nil && !errors.Is(err, logger.ErrRecordNotFound):
		fields = append(fields, slog.Any("error", err))
		l.loggerInstance.ErrorContext(ctx, "gorm trace error", fields...)
	case l.SlowThreshold != 0 && elapsed > l.SlowThreshold:
		fields = append(fields, slog.String("type", "slow_query"))
		l.loggerInstance.WarnContext(ctx, "gorm trace slow query", fields...)
	default:
		l.loggerInstance.DebugContext(ctx, "gorm trace", fields...)
	}
}
