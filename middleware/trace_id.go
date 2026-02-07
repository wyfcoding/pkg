// Package middleware 提供了通用的 Gin 与 gRPC 中间件实现。
// 生成摘要:
// 1) 新增 Trace ID 响应头注入中间件，便于客户端关联日志。
// 假设:
// 1) Trace ID 从 OpenTelemetry 上下文中获取。
package middleware

import (
	"github.com/wyfcoding/pkg/tracing"

	"github.com/gin-gonic/gin"
)

// HeaderXTraceID 定义 Trace ID 响应头名称。
const HeaderXTraceID = "X-Trace-ID"

// TraceIDHeader 返回一个 Gin 中间件，用于注入 Trace ID 响应头。
func TraceIDHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := tracing.GetTraceID(c.Request.Context())
		if traceID != "" {
			c.Header(HeaderXTraceID, traceID)
		}

		c.Next()

		if traceID == "" {
			traceID = tracing.GetTraceID(c.Request.Context())
			if traceID != "" && !c.Writer.Written() {
				c.Header(HeaderXTraceID, traceID)
			}
		}
	}
}
