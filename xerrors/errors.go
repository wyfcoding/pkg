package xerrors

import (
	"fmt"
	"net/http"
	"runtime"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorType 错误的大类
type ErrorType uint

const (
	ErrUnknown ErrorType = iota
	ErrInternal
	ErrInvalidArg
	ErrNotFound
	ErrAlreadyExists
	ErrPermissionDenied
	ErrUnauthenticated
	ErrDeadlineExceeded
	ErrUnavailable
	ErrLimitExceeded
)

// Error 增强型错误结构
type Error struct {
	Type    ErrorType      `json:"type"`
	Code    int            `json:"code"`    // 业务自定义错误码
	Message string         `json:"message"` // 对外展示的友好消息
	Detail  string         `json:"detail"`  // 对内调试的详细信息
	Cause   error          `json:"-"`       // 原始错误
	Stack   []string       `json:"stack"`   // 堆栈追踪
	Context map[string]any `json:"context"` // 上下文数据 (UserID, ID 等)
}

// Error 实现 error 接口
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %d: %s (Cause: %v)", e.Type.String(), e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %d: %s", e.Type.String(), e.Code, e.Message)
}

// Unwrap 实现 Go 1.13 解包接口
func (e *Error) Unwrap() error {
	return e.Cause
}

func (t ErrorType) String() string {
	return [...]string{
		"Unknown", "Internal", "InvalidArg", "NotFound", "AlreadyExists",
		"PermissionDenied", "Unauthenticated", "DeadlineExceeded", "Unavailable", "LimitExceeded",
	}[t]
}

// --- 核心构造函数 ---

// New 创建新错误并自动捕获堆栈
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

// captureStack 捕获当前调用栈 (深度限制 10 层)
func (e *Error) captureStack() {
	const depth = 10
	var pcs [depth]uintptr
	n := runtime.Callers(3, pcs[:]) // 跳过 captureStack, New 和上层构造函数
	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()
		// 仅保留关键路径信息：文件名:行号 (函数名)
		e.Stack = append(e.Stack, fmt.Sprintf("%s:%d (%s)", frame.File, frame.Line, frame.Function))
		if !more || len(e.Stack) >= depth {
			break
		}
	}
}

// --- 链式 API ---

func (e *Error) WithContext(key string, value any) *Error {
	e.Context[key] = value
	return e
}

func (e *Error) WithDetail(format string, args ...any) *Error {
	e.Detail = fmt.Sprintf(format, args...)
	return e
}

// --- 快捷构造工具 ---

func Internal(msg string, cause error) *Error {
	return New(ErrInternal, 500, msg, "", cause)
}

func InvalidArg(msg string) *Error {
	return New(ErrInvalidArg, 400, msg, "", nil)
}

func NotFound(msg string) *Error {
	return New(ErrNotFound, 404, msg, "", nil)
}

func Unauthenticated(msg string) *Error {
	return New(ErrUnauthenticated, 401, msg, "", nil)
}

// Wrap 包装现有错误并捕获堆栈
func Wrap(err error, errType ErrorType, msg string) *Error {
	if err == nil {
		return nil
	}
	// 如果已经是 *Error 类型，则保持其原始类型和堆栈，仅更新 Message 和 Cause
	if e, ok := FromError(err); ok {
		e.Cause = err
		e.Message = msg
		return e
	}
	return New(errType, int(errType), msg, "", err)
}

// WrapInternal 快速包装内部服务器错误
func WrapInternal(err error, msg string) *Error {
	return Wrap(err, ErrInternal, msg)
}

// --- 协议转换 ---

// HTTPStatus 自动映射 HTTP 状态码
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

// GRPCCode 自动映射 gRPC 状态码
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

// ToGRPCStatus 将 Error 转换为带 Details 的 gRPC Status
func (e *Error) ToGRPCStatus() *status.Status {
	st := status.New(e.GRPCCode(), e.Message)
	// 这里可以进一步使用 st.WithDetails(...) 注入结构化错误数据
	return st
}

// FromError 尝试转换
func FromError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	e, ok := err.(*Error)
	return e, ok
}
