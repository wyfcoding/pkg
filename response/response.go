// Package response 提供了统一的 HTTP 响应封装，支持业务错误码映射、分页数据包装及 gRPC 状态码转换。
// 生成摘要:
// 1) Error 响应优先识别 xerrors 并输出规范化消息与详情字段。
// 2) 错误与成功响应附带 request_id 与 trace_id，便于链路定位。
// 3) 错误响应在识别 xerrors 时附带 error_type 与 error_code。
// 假设:
// 1) xerrors.Message 为对外可读的业务错误描述。
package response

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/wyfcoding/pkg/contextx"
	"github.com/wyfcoding/pkg/tracing"
	"github.com/wyfcoding/pkg/xerrors"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HTTPStatusProvider 定义了能够提供 HTTP 状态码的错误接口。
type HTTPStatusProvider interface {
	HTTPStatus() int // 返回对应的 HTTP 标准状态码。
}

// Success 发送一个标准的成功响应。
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, withMeta(c, gin.H{
		"code": 0,
		"msg":  "success",
		"data": data,
	}))
}

// SuccessWithMsg 发送一个带自定义消息的成功响应.
func SuccessWithMsg(c *gin.Context, msg string, data any) {
	c.JSON(http.StatusOK, withMeta(c, gin.H{
		"code": 0,
		"msg":  msg,
		"data": data,
	}))
}

// SuccessWithStatus 发送一个带有指定 HTTP 状态码和消息的成功响应.
func SuccessWithStatus(c *gin.Context, statusVal int, msg string, data any) {
	c.JSON(statusVal, withMeta(c, gin.H{
		"code": 0,
		"msg":  msg,
		"data": data,
	}))
}

// SuccessWithPagination 发送一个包含分页信息的成功响应。
func SuccessWithPagination(c *gin.Context, data any, total int64, page, size int32) {
	c.JSON(http.StatusOK, withMeta(c, gin.H{
		"code":  0,
		"msg":   "success",
		"data":  data,
		"total": total,
		"page":  page,
		"size":  size,
	}))
}

// SuccessWithRawData 发送原始数据的成功响应。
func SuccessWithRawData(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// Error 发送智能错误响应。
func Error(c *gin.Context, err error) {
	if err == nil {
		Success(c, nil)

		return
	}

	statusCode := http.StatusInternalServerError
	msg := err.Error()
	detail := ""
	errorType := ""
	errorCode := 0

	// 1. 优先识别统一错误类型，避免直接暴露内部堆栈。
	if xe, ok := xerrors.FromError(err); ok {
		statusCode = xe.HTTPStatus()
		if xe.Message != "" {
			msg = xe.Message
		}
		if xe.Detail != "" {
			detail = xe.Detail
		}
		errorType = xe.Type.String()
		errorCode = xe.Code
	} else if errors.Is(err, context.DeadlineExceeded) {
		statusCode = http.StatusGatewayTimeout
		msg = "Request Timeout"
	} else if errors.Is(err, context.Canceled) {
		statusCode = 499
		msg = "Client Closed Request"
	} else if provider, ok := err.(HTTPStatusProvider); ok {
		// 2. 兼容自定义 HTTP 状态码提供者。
		statusCode = provider.HTTPStatus()
	} else if st, ok := status.FromError(err); ok {
		// 3. 处理 gRPC 返回的远程调用错误，映射为标准 HTTP 状态码。
		statusCode = grpcCodeToHTTP(st.Code())
		msg = st.Message()
	}

	// 自动记录 5xx 错误日志
	if statusCode >= 500 {
		slog.ErrorContext(c.Request.Context(), "internal_server_error",
			"error", err,
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
		)
	}

	payload := gin.H{
		"code":   statusCode,
		"msg":    msg,
		"detail": detail,
	}
	if errorType != "" {
		payload["error_type"] = errorType
	}
	if errorCode != 0 {
		payload["error_code"] = errorCode
	}

	c.JSON(statusCode, withMeta(c, payload))
}

// ErrorWithStatus 发送一个带有指定 HTTP 状态码、消息和详情的错误响应。
func ErrorWithStatus(c *gin.Context, statusVal int, msg, detail string) {
	c.JSON(statusVal, withMeta(c, gin.H{
		"code":       statusVal,
		"msg":        msg,
		"detail":     detail,
		"request_id": contextx.GetRequestID(c.Request.Context()),
		"trace_id":   xerrorsTraceID(c.Request.Context()),
	}))
}

func xerrorsTraceID(ctx context.Context) string {
	return tracing.GetTraceID(ctx)
}

func withMeta(c *gin.Context, payload gin.H) gin.H {
	if payload == nil {
		payload = gin.H{}
	}

	if _, ok := payload["request_id"]; !ok {
		payload["request_id"] = contextx.GetRequestID(c.Request.Context())
	}
	if _, ok := payload["trace_id"]; !ok {
		payload["trace_id"] = xerrorsTraceID(c.Request.Context())
	}

	return payload
}

func grpcCodeToHTTP(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		const clientClosedRequest = 499
		return clientClosedRequest
	case codes.Unknown:
		return http.StatusInternalServerError
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusBadRequest
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Code(12):
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DataLoss:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
