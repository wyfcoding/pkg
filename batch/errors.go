package batch

import "errors"

var (
	ErrBufferClosed     = errors.New("buffer is closed")
	ErrBufferFull       = errors.New("buffer is full")
	ErrHandlerNil       = errors.New("handler cannot be nil")
	ErrInvalidConfig    = errors.New("invalid configuration")
	ErrFlushTimeout     = errors.New("flush timeout")
	ErrTransformFailed  = errors.New("transform failed")
)
