package tracing

import (
	"context"
	"fmt"

	"github.com/wyfcoding/pkg/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	// Ensure these are imported if used by config.TracingConfig
	// "google.golang.org/grpc"
	// "google.golang.org/grpc/credentials/insecure"
)

// InitTracer 使用 OTLP 导出器初始化 OpenTelemetry 追踪器。
// 它配置了服务名、导出器、资源和采样策略。
// cfg 参数应包含 OTLP 端点和服务的名称。
// 返回一个函数，该函数在调用时会优雅地关闭追踪提供者。
func InitTracer(cfg config.TracingConfig) (func(context.Context) error, error) {
	ctx := context.Background()
	// 创建OTLP gRPC追踪导出器。
	// WithInsecure 表示使用非加密连接，生产环境应考虑使用TLS。
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create otlp exporter: %w", err)
	}

	// 创建资源，用于标识生成追踪数据的服务。
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// 创建TracerProvider，并配置其批量导出器、资源和采样策略。
	// sdktrace.AlwaysSample() 表示所有追踪都将被采样，根据需要可以调整为其他采样策略。
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // 根据需要调整采样策略
	)

	// 设置全局的TracerProvider。
	otel.SetTracerProvider(tp)
	// 设置全局的TextMapPropagator，用于在服务边界间传播追踪上下文。
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// 返回关闭函数，确保在应用关闭时能优雅地刷新和关闭追踪。
	return tp.Shutdown, nil
}
