package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/wyfcoding/pkg/async"
)

var (
	ErrPoolClosed = errors.New("worker pool is closed")
)

// Task 是 worker 执行的任务函数。
type Task func(ctx context.Context)

// Pool 是一个通用的 worker 池。
type Pool struct {
	tasks   chan Task
	quit    chan struct{}
	options *poolOptions
	wg      sync.WaitGroup
	closed  int32
	active  int32 // 当前活跃的 worker 数量
}

type poolOptions struct {
	Logger       *slog.Logger
	PanicHandler func(any)
	Name         string
	Size         int
	QueueSize    int
}

// Option 定义配置选项。
type Option func(*poolOptions)

// WithName 设置池名称。
func WithName(name string) Option {
	return func(o *poolOptions) {
		o.Name = name
	}
}

// WithSize 设置 worker 数量。
func WithSize(size int) Option {
	return func(o *poolOptions) {
		o.Size = size
	}
}

// WithQueueSize 设置任务队列大小。
func WithQueueSize(size int) Option {
	return func(o *poolOptions) {
		o.QueueSize = size
	}
}

// NewPool 创建一个新的 worker 池。
func NewPool(opts ...Option) *Pool {
	options := &poolOptions{
		Name:      "default-pool",
		Size:      10,
		QueueSize: 100,
		Logger:    slog.Default(),
	}
	for _, opt := range opts {
		opt(options)
	}

	p := &Pool{
		tasks:   make(chan Task, options.QueueSize),
		quit:    make(chan struct{}),
		options: options,
	}

	p.start()
	return p
}

func (p *Pool) start() {
	p.options.Logger.Info("Worker pool starting", "name", p.options.Name, "size", p.options.Size)
	for range p.options.Size {
		p.wg.Add(1)
		atomic.AddInt32(&p.active, 1)
		async.SafeGo(func() {
			defer p.wg.Done()
			defer atomic.AddInt32(&p.active, -1)
			p.runWorker()
		})
	}
}

func (p *Pool) runWorker() {
	for {
		select {
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			// 执行任务，context 目前使用 Background，也可以从 Task 传入
			task(context.Background())
		case <-p.quit:
			return
		}
	}
}

// Submit 提交一个任务。如果池已满且 blocking 为 true，则阻塞；否则返回错误（需扩展 error）。
// 当前实现为阻塞提交，直到队列有空位。
func (p *Pool) Submit(task Task) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return ErrPoolClosed
	}

	select {
	case p.tasks <- task:
		return nil
	case <-p.quit:
		return ErrPoolClosed
	}
}

// Stop 停止 worker 池，等待所有正在执行的任务完成。
func (p *Pool) Stop() {
	if !atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		return
	}
	close(p.quit)  // 通知 worker 退出
	p.wg.Wait()    // 等待所有 worker 退出
	close(p.tasks) // 关闭任务通道
	p.options.Logger.Info("Worker pool stopped", "name", p.options.Name)
}
