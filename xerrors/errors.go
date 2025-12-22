// Package xerrors 提供了增强的错误处理机制，统一了错误码、HTTP状态码和gRPC状态码的映射，
// 并支持错误包装、堆栈跟踪（预留接口）和多错误类型分类。
// 旨在替代标准库 errors 和第三方库，成为项目内唯一的错误处理标准。
package xerrors

import (
	"fmt"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Code 定义了业务错误码类型。
// 业务错误码不同于 HTTP 状态码，它通常是更细粒度的整型标识，
// 例如 100101 表示 "用户余额不足"。
type Code int

// ErrorType 定义了错误的通用类别（Category）。
// 用于快速判断错误性质，例如是否为 "未找到"、"权限不足" 等，方便中间件进行统一处理。
type ErrorType uint

const (
	// ErrUnknown 未知错误
	ErrUnknown ErrorType = iota
	// ErrNotFound 资源未找到
	ErrNotFound
	// ErrInvalidArg 参数无效
	ErrInvalidArg
	// ErrAlreadyExists 资源已存在
	ErrAlreadyExists
	// ErrPermissionDenied 权限不足 (Forbidden)
	ErrPermissionDenied
	// ErrUnauthenticated 未认证 (Unauthorized)
	ErrUnauthenticated
	// ErrInternal 服务器内部错误
	ErrInternal
	// ErrDeadlineExceeded 超时
	ErrDeadlineExceeded
	// ErrUnavailable 服务不可用
	ErrUnavailable
	// ErrLimitExceeded 限流/配额超限
	ErrLimitExceeded
)

// Error 是项目内统一的错误接口实现。
type Error struct {
	Type    ErrorType      // 错误类别
	Code    int            // 业务错误码 (可选，0表示无特定业务码)
	Message string         // 对外展示的错误信息 (User Message)
	Cause   error          // 根本原因 (Root Cause)，通常是底层库返回的 error
	Context map[string]any // 错误上下文，用于记录额外信息（如 UserID, OrderID）
}

// Error 实现 error 接口。
// 返回格式: "[Type] Message: Cause"
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type.String(), e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Type.String(), e.Message)
}

// Unwrap 实现 Go 1.13+ 的 Unwrap 接口，支持 errors.Is 和 errors.As。
func (e *Error) Unwrap() error {
	return e.Cause
}

// String 实现 fmt.Stringer 接口。
func (t ErrorType) String() string {
	switch t {
	case ErrNotFound:
		return "NotFound"
	case ErrInvalidArg:
		return "InvalidArg"
	case ErrAlreadyExists:
		return "AlreadyExists"
	case ErrPermissionDenied:
		return "PermissionDenied"
	case ErrUnauthenticated:
		return "Unauthenticated"
	case ErrInternal:
		return "Internal"
	case ErrDeadlineExceeded:
		return "DeadlineExceeded"
	case ErrUnavailable:
		return "Unavailable"
	case ErrLimitExceeded:
		return "LimitExceeded"
	default:
		return "Unknown"
	}
}

// New 创建一个新的 Error。
func New(errType ErrorType, msg string, cause error) *Error {
	return &Error{
		Type:    errType,
		Message: msg,
		Cause:   cause,
		Context: make(map[string]any),
	}
}

// WithCode 为错误附加业务错误码。
func (e *Error) WithCode(code int) *Error {
	e.Code = code
	return e
}

// WithContext 为错误附加上下文信息。
func (e *Error) WithContext(key string, value any) *Error {
	e.Context[key] = value
	return e
}

// 常用错误构造函数

// NewNotFound 创建一个 ErrNotFound 错误。
func NewNotFound(format string, args ...any) error {
	return New(ErrNotFound, fmt.Sprintf(format, args...), nil)
}

// NewInvalidArg 创建一个 ErrInvalidArg 错误。
func NewInvalidArg(format string, args ...any) error {
	return New(ErrInvalidArg, fmt.Sprintf(format, args...), nil)
}

// NewInternal 创建一个 ErrInternal 错误。
func NewInternal(cause error, format string, args ...any) error {
	return New(ErrInternal, fmt.Sprintf(format, args...), cause)
}

// NewAlreadyExists 创建一个 ErrAlreadyExists 错误。
func NewAlreadyExists(format string, args ...any) error {
	return New(ErrAlreadyExists, fmt.Sprintf(format, args...), nil)
}

// NewPermissionDenied 创建一个 ErrPermissionDenied 错误。
func NewPermissionDenied(format string, args ...any) error {
	return New(ErrPermissionDenied, fmt.Sprintf(format, args...), nil)
}

// NewUnauthenticated 创建一个 ErrUnauthenticated 错误。
func NewUnauthenticated(format string, args ...any) error {
	return New(ErrUnauthenticated, fmt.Sprintf(format, args...), nil)
}

// HTTPStatus 将错误转换为 HTTP 状态码。
func HTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}

	// 尝试解包
	if appErr, ok := err.(*Error); ok {
		switch appErr.Type {
		case ErrNotFound:
			return http.StatusNotFound
		case ErrInvalidArg:
			return http.StatusBadRequest
		case ErrAlreadyExists:
			return http.StatusConflict
		case ErrPermissionDenied:
			return http.StatusForbidden
		case ErrUnauthenticated:
			return http.StatusUnauthorized
		case ErrDeadlineExceeded:
			return http.StatusGatewayTimeout
		case ErrUnavailable:
			return http.StatusServiceUnavailable
		case ErrLimitExceeded:
			return http.StatusTooManyRequests
		case ErrInternal:
			return http.StatusInternalServerError
		default:
			return http.StatusInternalServerError
		}
	}

	return http.StatusInternalServerError
}

// GRPCCode 将错误转换为 gRPC 状态码。
func GRPCCode(err error) codes.Code {
	if err == nil {
		return codes.OK
	}

	if appErr, ok := err.(*Error); ok {
		switch appErr.Type {
		case ErrNotFound:
			return codes.NotFound
		case ErrInvalidArg:
			return codes.InvalidArgument
		case ErrAlreadyExists:
			return codes.AlreadyExists
		case ErrPermissionDenied:
			return codes.PermissionDenied
		case ErrUnauthenticated:
			return codes.Unauthenticated
		case ErrDeadlineExceeded:
			return codes.DeadlineExceeded
		case ErrUnavailable:
			return codes.Unavailable
		case ErrLimitExceeded:
			return codes.ResourceExhausted
		case ErrInternal:
			return codes.Internal
		default:
			return codes.Internal
		}
	}
	return codes.Internal
}

// ToGRPCError 将错误转换为标准的 gRPC error。
// 客户端接收到此错误后，可以使用 status.FromError 解析出 Code 和 Message。
func ToGRPCError(err error) error {
	if err == nil {
		return nil
	}
	// 如果已经是 gRPC status error，直接返回
	if _, ok := status.FromError(err); ok {
		return err
	}

	// 否则根据 ErrorType 转换
	return status.Error(GRPCCode(err), err.Error())
}

// FromError 尝试将任意 error 转换为 *Error。
// 如果已经是 *Error，直接返回；否则包装为 ErrInternal。
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*Error); ok {
		return e
	}
	return New(ErrInternal, err.Error(), err)
}
