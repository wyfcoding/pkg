package tracing

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wyfcoding/pkg/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

// InitTracer 深度优化版的追踪初始化
func InitTracer(cfg config.TracingConfig) (func(context.Context) error, error) {
	ctx := context.Background()

	// 1. 导出器配置 (OTLP gRPC)
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create otlp exporter: %w", err)
	}

	// 2. 资源描述 (增加环境标签)
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			attribute.String("exporter", "otlp"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// 3. 采样策略：ParentBased + Ratio
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))

	// 4. 提供者配置
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// 5. 设置全局属性
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("Tracer provider initialized", "service", cfg.ServiceName, "endpoint", cfg.OTLPEndpoint)

	return tp.Shutdown, nil
}

// --- 生产级辅助工具 API ---

// StartSpan 开启一个子 Span 的便捷方式
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.Tracer("github.com/wyfcoding/pkg/tracing")
	return tracer.Start(ctx, name, opts...)
}

// AddTag 为当前 Span 快速添加业务属性 (修复了 attribute.Any 不存在的问题)
func AddTag(ctx context.Context, key string, value any) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	// 根据类型选择合适的 attribute 函数
	switch v := value.(type) {
	case string:
		span.SetAttributes(attribute.String(key, v))
	case int:
		span.SetAttributes(attribute.Int(key, v))
	case int64:
		span.SetAttributes(attribute.Int64(key, v))
	case bool:
		span.SetAttributes(attribute.Bool(key, v))
	case float64:
		span.SetAttributes(attribute.Float64(key, v))
	default:
		span.SetAttributes(attribute.String(key, fmt.Sprintf("%v", v)))
	}
}

// SetError 为当前 Span 标记错误并记录详情 (修复了 SetStatus 参数错误问题)
func SetError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	// 正确用法：第一个参数是 codes.Code，第二个参数是描述信息
	span.SetStatus(codes.Error, err.Error())
}

// GetTraceID 获取当前上下文中的 Trace ID
func GetTraceID(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		return spanCtx.TraceID().String()
	}
	return ""
}

// InjectContext 将当前上下文中的追踪信息注入到 Map 中
func InjectContext(ctx context.Context) map[string]string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier
}

// ExtractContext 从 Map 中提取追踪信息并返回新的上下文
func ExtractContext(ctx context.Context, carrier map[string]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(carrier))
}
