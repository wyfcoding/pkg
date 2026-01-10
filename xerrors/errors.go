// Package xerrors 提供了具备类型分类、堆栈追踪、上下文关联及多协议（HTTP/gRPC）状态映射能力的增强型错误系统.
package xerrors

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorType 定义了错误的大类.
type ErrorType uint

const (
	// ErrUnknown 未知错误.
	ErrUnknown ErrorType = iota
	// ErrInternal 内部服务器错误.
	ErrInternal
	// ErrInvalidArg 参数校验失败.
	ErrInvalidArg
	// ErrNotFound 资源不存在.
	ErrNotFound
	// ErrAlreadyExists 资源已存在（冲突）.
	ErrAlreadyExists
	// ErrPermissionDenied 权限不足.
	ErrPermissionDenied
	// ErrUnauthenticated 未经过身份认证.
	ErrUnauthenticated
	// ErrDeadlineExceeded 操作超时.
	ErrDeadlineExceeded
	// ErrUnavailable 服务不可用.
	ErrUnavailable
	// ErrLimitExceeded 触发限流.
	ErrLimitExceeded
)

// Error 结构体封装了详细的错误上下文信息.
// 优化：重新排序字段以优化内存对齐 (fieldalignment).
type Error struct {
	Context map[string]any
	Cause   error
	Message string
	Detail  string
	Stack   []string
	Code    int
	Type    ErrorType
}

// Error 实现 error 接口.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %d: %s (cause: %v)", e.Type.String(), e.Code, e.Message, e.Cause)
	}

	return fmt.Sprintf("[%s] %d: %s", e.Type.String(), e.Code, e.Message)
}

// Unwrap 实现 Go 1.13 解包接口.
func (e *Error) Unwrap() error {
	return e.Cause
}

func (t ErrorType) String() string {
	names := [...]string{
		"Unknown", "Internal", "InvalidArg", "NotFound", "AlreadyExists",
		"PermissionDenied", "Unauthenticated", "DeadlineExceeded", "Unavailable", "LimitExceeded",
	}

	if t < ErrorType(len(names)) {
		return names[t]
	}

	return "Unknown"
}

// New 构造一个新的增强型错误对象.
func New(errType ErrorType, code int, message, detail string, cause error) *Error {
	e := &Error{
		Type:    errType,
		Code:    code,
		Message: message,
		Detail:  detail,
		Cause:   cause,
		Context: make(map[string]any),
		Stack:   nil,
	}
	e.captureStack()

	return e
}

const (
	maxStackDepth = 10
	skipCallers   = 3
)

func (e *Error) captureStack() {
	var pcs [maxStackDepth]uintptr
	n := runtime.Callers(skipCallers, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()
		e.Stack = append(e.Stack, fmt.Sprintf("%s:%d (%s)", frame.File, frame.Line, frame.Function))
		if !more || len(e.Stack) >= maxStackDepth {
			break
		}
	}
}

// WithContext 为错误注入额外的业务上下文数据.
func (e *Error) WithContext(key string, value any) *Error {
	e.Context[key] = value

	return e
}

// WithDetail 为错误注入详细的内部调试信息.
func (e *Error) WithDetail(format string, args ...any) *Error {
	e.Detail = fmt.Sprintf(format, args...)

	return e
}

const (
	httpInternalError = 500
	httpBadRequest    = 400
	httpNotFound      = 404
	httpUnauthorized  = 401
)

// Internal 快捷构造内部错误.
func Internal(msg string, cause error) *Error {
	return New(ErrInternal, httpInternalError, msg, "", cause)
}

// InvalidArg 快捷构造参数错误.
func InvalidArg(msg string) *Error {
	return New(ErrInvalidArg, httpBadRequest, msg, "", nil)
}

// NotFound 快捷构造 404 错误.
func NotFound(msg string) *Error {
	return New(ErrNotFound, httpNotFound, msg, "", nil)
}

// Unauthenticated 快捷构造未认证错误.
func Unauthenticated(msg string) *Error {
	return New(ErrUnauthenticated, httpUnauthorized, msg, "", nil)
}

// Wrap 包装现有错误并捕获堆栈.
func Wrap(err error, errType ErrorType, msg string) *Error {
	if err == nil {
		return nil
	}

	if e, ok := FromError(err); ok {
		e.Cause = err
		e.Message = msg

		return e
	}

	// 显式范围检查以消除 G115.
	t := uint64(errType)
	var errCode uint32
	if t <= 0xFFFFFFFF {
		errCode = uint32(t)
	} else {
		errCode = 0
	}
	return New(errType, int(errCode), msg, "", err)
}

// WrapInternal 快速包装内部服务器错误.
func WrapInternal(err error, msg string) *Error {
	return Wrap(err, ErrInternal, msg)
}

// HTTPStatus 实现了 response.HTTPStatusProvider 接口.
func (e *Error) HTTPStatus() int {
	switch e.Type {
	case ErrInvalidArg:
		return http.StatusBadRequest
	case ErrUnauthenticated:
		return http.StatusUnauthorized
	case ErrPermissionDenied:
		return http.StatusForbidden
	case ErrNotFound:
		return http.StatusNotFound
	case ErrAlreadyExists:
		return http.StatusConflict
	case ErrLimitExceeded:
		return http.StatusTooManyRequests
	case ErrDeadlineExceeded:
		return http.StatusGatewayTimeout
	case ErrUnknown, ErrInternal, ErrUnavailable:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// GRPCCode 执行错误类型到 gRPC 标准状态码的映射.
func (e *Error) GRPCCode() codes.Code {
	switch e.Type {
	case ErrInvalidArg:
		return codes.InvalidArgument
	case ErrUnauthenticated:
		return codes.Unauthenticated
	case ErrPermissionDenied:
		return codes.PermissionDenied
	case ErrNotFound:
		return codes.NotFound
	case ErrAlreadyExists:
		return codes.AlreadyExists
	case ErrLimitExceeded:
		return codes.ResourceExhausted
	case ErrDeadlineExceeded:
		return codes.DeadlineExceeded
	case ErrUnavailable:
		return codes.Unavailable
	case ErrUnknown, ErrInternal:
		return codes.Internal
	default:
		return codes.Internal
	}
}

// ToGRPCStatus 将 Error 转换为带 Details 的 gRPC Status.
func (e *Error) ToGRPCStatus() *status.Status {
	return status.New(e.GRPCCode(), e.Message)
}

// FromError 尝试 from error 类型转换回 *Error.
func FromError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}

	var e *Error
	if errors.As(err, &e) {
		return e, true
	}

	return nil, false
}
