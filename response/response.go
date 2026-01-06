// Package response 提供了统一的 HTTP 响应封装，支持业务错误码映射、分页数据包装及 gRPC 状态码转换。
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HTTPStatusProvider 定义了能够提供 HTTP 状态码的错误接口。
// 用于支持跨层级的错误透传与状态码自动映射。
type HTTPStatusProvider interface {
	HTTPStatus() int // 返回对应的 HTTP 标准状态码
}

// Success 发送一个标准的成功响应。
// 默认：HTTP 200，业务码 0，消息 "success"。
func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": data,
	})
}

// SuccessWithStatus 发送一个带有指定 HTTP 状态码和消息的成功响应。
func SuccessWithStatus(c *gin.Context, status int, msg string, data any) {
	c.JSON(status, gin.H{
		"code": 0,
		"msg":  "success",
		"data": data,
	})
}

// SuccessWithPagination 发送一个包含分页信息的成功响应。
func SuccessWithPagination(c *gin.Context, data any, total int64, page, size int32) {
	c.JSON(http.StatusOK, gin.H{
		"code":  0,
		"msg":   "success",
		"data":  data,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// SuccessWithRawData 发送原始数据的成功响应 (不包装 code 和 msg)。
// 用于某些特定系统接口 (如 Health Check)。
func SuccessWithRawData(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// Error 发送智能错误响应。
// 核心逻辑：自动识别 pkg/xerrors (业务错误) 或 gRPC Status (RPC 错误) 并执行状态码映射。
// 若无法识别类型，则兜底返回 500 Internal Server Error。
func Error(c *gin.Context, err error) {
	if err == nil {
		Success(c, nil)
		return
	}

	statusCode := http.StatusInternalServerError
	msg := err.Error()
	detail := ""

	// 1. 优先尝试从业务错误接口获取状态码
	if e, ok := err.(HTTPStatusProvider); ok {
		statusCode = e.HTTPStatus()
	} else if st, ok := status.FromError(err); ok {
		// 2. 处理 gRPC 返回的远程调用错误，映射为标准 HTTP 状态码
		statusCode = grpcCodeToHTTP(st.Code())
		msg = st.Message()
	}

	c.JSON(statusCode, gin.H{
		"code":   statusCode,
		"msg":    msg,
		"detail": detail,
	})
}

// ErrorWithStatus 发送一个带有指定 HTTP 状态码、消息和详情的错误响应。
func ErrorWithStatus(c *gin.Context, status int, msg string, detail string) {
	c.JSON(status, gin.H{
		"code":   status,
		"msg":    msg,
		"detail": detail,
	})
}

// grpcCodeToHTTP 执行 gRPC 到 HTTP 的标准协议映射。
func grpcCodeToHTTP(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return 499 // Client Closed Request
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
	case codes.Unimplemented:
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
