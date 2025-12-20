package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// TracingMiddleware 返回一个中间件，用于向请求添加 OpenTelemetry 追踪。
// 它包装了 otelgin.Middleware 以提供标准的追踪能力。
// serviceName: 服务名称，用于标识 Span 的来源。
func TracingMiddleware(serviceName string) gin.HandlerFunc {
	return otelgin.Middleware(serviceName)
}
