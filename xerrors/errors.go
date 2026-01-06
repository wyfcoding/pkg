// Package xerrors 提供了具备类型分类、堆栈追踪、上下文关联及多协议（HTTP/gRPC）状态映射能力的增强型错误系统。
package xerrors

import (
	"fmt"
	"net/http"
	"runtime"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorType 定义了错误的大类，用于跨协议的状态码映射逻辑。
type ErrorType uint

const (
	ErrUnknown          ErrorType = iota // 未知错误
	ErrInternal                          // 内部服务器错误
	ErrInvalidArg                        // 参数校验失败
	ErrNotFound                          // 资源不存在
	ErrAlreadyExists                     // 资源已存在（冲突）
	ErrPermissionDenied                  // 权限不足
	ErrUnauthenticated                   // 未经过身份认证
	ErrDeadlineExceeded                  // 操作超时
	ErrUnavailable                       // 服务不可用
	ErrLimitExceeded                     // 触发限流
)

// Error 结构体封装了详细的错误上下文信息。
type Error struct {
	Type    ErrorType      `json:"type"`    // 错误分类
	Code    int            `json:"code"`    // 业务层面的自定义错误码
	Message string         `json:"message"` // 面向前端或用户的友好提示消息
	Detail  string         `json:"detail"`  // 面向开发者的详细调试信息
	Cause   error          `json:"-"`       // 底层原始错误（不参与序列化）
	Stack   []string       `json:"stack"`   // 自动捕获的函数调用栈
	Context map[string]any `json:"context"` // 相关的业务上下文数据 (如 userID, orderID)
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %d: %s (cause: %v)", e.Type.String(), e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %d: %s", e.Type.String(), e.Code, e.Message)
}

// Unwrap 实现 Go 1.13 解包接口。
func (e *Error) Unwrap() error {
	return e.Cause
}

func (t ErrorType) String() string {
	return [...]string{
		"Unknown", "Internal", "InvalidArg", "NotFound", "AlreadyExists",
		"PermissionDenied", "Unauthenticated", "DeadlineExceeded", "Unavailable", "LimitExceeded",
	}[t]
}

// New 构造一个新的增强型错误对象，并自动捕获当前的调用堆栈。
func New(errType ErrorType, code int, message string, detail string, cause error) *Error {
	e := &Error{
		Type:    errType,
		Code:    code,
		Message: message,
		Detail:  detail,
		Cause:   cause,
		Context: make(map[string]any),
	}
	e.captureStack()
	return e
}

// captureStack 提取当前代码的运行路径信息，辅助快速定位线上问题。
func (e *Error) captureStack() {
	const depth = 10
	var pcs [depth]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()
		e.Stack = append(e.Stack, fmt.Sprintf("%s:%d (%s)", frame.File, frame.Line, frame.Function))
		if !more || len(e.Stack) >= depth {
			break
		}
	}
}

// WithContext 为错误注入额外的业务上下文数据。
func (e *Error) WithContext(key string, value any) *Error {
	e.Context[key] = value
	return e
}

// WithDetail 为错误注入详细的内部调试信息。
func (e *Error) WithDetail(format string, args ...any) *Error {
	e.Detail = fmt.Sprintf(format, args...)
	return e
}

// Internal 快捷构造内部错误。
func Internal(msg string, cause error) *Error {
	return New(ErrInternal, 500, msg, "", cause)
}

// InvalidArg 快捷构造参数错误。
func InvalidArg(msg string) *Error {
	return New(ErrInvalidArg, 400, msg, "", nil)
}

// NotFound 快捷构造 404 错误。
func NotFound(msg string) *Error {
	return New(ErrNotFound, 404, msg, "", nil)
}

// Unauthenticated 快捷构造未认证错误。
func Unauthenticated(msg string) *Error {
	return New(ErrUnauthenticated, 401, msg, "", nil)
}

// Wrap 包装现有错误并捕获堆栈。
func Wrap(err error, errType ErrorType, msg string) *Error {
	if err == nil {
		return nil
	}
	if e, ok := FromError(err); ok {
		e.Cause = err
		e.Message = msg
		return e
	}
	return New(errType, int(errType), msg, "", err)
}

// WrapInternal 快速包装内部服务器错误。
func WrapInternal(err error, msg string) *Error {
	return Wrap(err, ErrInternal, msg)
}

// HTTPStatus 实现了 response.HTTPStatusProvider 接口，执行错误类型到 HTTP 协议的自动映射。
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
	default:
		return http.StatusInternalServerError
	}
}

// GRPCCode 执行错误类型到 gRPC 标准状态码的映射。
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
	default:
		return codes.Internal
	}
}

// ToGRPCStatus 将 Error 转换为带 Details 的 gRPC Status。
func (e *Error) ToGRPCStatus() *status.Status {
	st := status.New(e.GRPCCode(), e.Message)
	return st
}

// FromError 尝试从 error 类型转换回 *Error。
func FromError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	e, ok := err.(*Error)
	return e, ok
}
