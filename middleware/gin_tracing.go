package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// TracingMiddleware returns a middleware that adds OpenTelemetry tracing to the request.
// It wraps otelgin.Middleware to provide standard tracing capabilities.
// serviceName: The name of the service, used to identify the source of the Spans.
func TracingMiddleware(serviceName string) gin.HandlerFunc {
	return otelgin.Middleware(serviceName)
}
