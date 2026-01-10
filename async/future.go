package async

import (
	"context"
	"sync"
)

// Future 代表一个异步计算的结果。
type Future[T any] struct {
	result T
	err    error
	done   chan struct{}
	once   sync.Once
}

// NewFuture 创建一个新的 Future。
// fn 是异步执行的函数，其结果将填充 Future。
func NewFuture[T any](fn func(ctx context.Context) (T, error)) *Future[T] {
	f := &Future[T]{
		done: make(chan struct{}),
	}
	SafeGo(func() {
		defer f.once.Do(func() { close(f.done) })
		// 注意：这里的 context 是通过闭包或外部传入控制，
		// 也可以扩展 NewFuture 接受 context。
		// 为简单起见，这里假设 fn 内部处理或使用 Background。
		res, err := fn(context.Background())
		f.result = res
		f.err = err
	})
	return f
}

// Get 阻塞等待计算完成并返回结果。
func (f *Future[T]) Get(ctx context.Context) (T, error) {
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case <-f.done:
		return f.result, f.err
	}
}
