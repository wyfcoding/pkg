package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/wyfcoding/pkg/async"
	"github.com/wyfcoding/pkg/metrics"
)

var (
	ErrPoolClosed  = errors.New("worker pool is closed")
	ErrPoolFull    = errors.New("worker pool is full")
	ErrTaskTimeout = errors.New("task submission timeout")
)

// Task 是 worker 执行的任务函数。
type Task func(ctx context.Context)

// Pool 是一个通用的 worker 池。
type Pool struct {
	tasks   chan Task
	quit    chan struct{}
	options *poolOptions
	metrics *workerMetrics
	wg      sync.WaitGroup
	closed  int32
	active  int32 // 当前活跃的 worker 数量
}

type workerMetrics struct {
	activeWorkers prometheus.Gauge
	queueLength   prometheus.Gauge
}

type poolOptions struct {
	Logger       *slog.Logger
	PanicHandler func(any)
	Metrics      *metrics.Metrics
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

// WithPanicHandler 设置 Panic 处理回调。
func WithPanicHandler(handler func(any)) Option {
	return func(o *poolOptions) {
		o.PanicHandler = handler
	}
}

// WithMetrics 注入指标采集器.
func WithMetrics(m *metrics.Metrics) Option {
	return func(o *poolOptions) {
		o.Metrics = m
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

	if options.Metrics != nil {
		p.metrics = &workerMetrics{
			activeWorkers: options.Metrics.NewGauge(&prometheus.GaugeOpts{
				Name:        "worker_pool_active_workers",
				Help:        "Number of active workers in the pool",
				ConstLabels: prometheus.Labels{"pool": options.Name},
			}),
			queueLength: options.Metrics.NewGauge(&prometheus.GaugeOpts{
				Name:        "worker_pool_queue_length",
				Help:        "Current length of the task queue",
				ConstLabels: prometheus.Labels{"pool": options.Name},
			}),
		}
	}

	p.start()
	return p
}

func (p *Pool) start() {
	p.options.Logger.Info("Worker pool starting", "name", p.options.Name, "size", p.options.Size)
	for range p.options.Size {
		p.wg.Add(1)
		atomic.AddInt32(&p.active, 1)
		if p.metrics != nil {
			p.metrics.activeWorkers.Inc()
		}
		async.SafeGo(func() {
			defer p.wg.Done()
			defer atomic.AddInt32(&p.active, -1)
			if p.metrics != nil {
				p.metrics.activeWorkers.Dec()
			}
			p.runWorker()
		})
	}
}

func (p *Pool) runWorker() {
	for {
		if p.metrics != nil {
			p.metrics.queueLength.Set(float64(len(p.tasks)))
		}
		select {
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			p.executeTask(task)
		case <-p.quit:
			return
		}
	}
}

func (p *Pool) executeTask(task Task) {
	defer func() {
		if r := recover(); r != nil {
			if p.options.PanicHandler != nil {
				p.options.PanicHandler(r)
			} else {
				p.options.Logger.Error("Worker task panic recovered", "panic", r)
			}
		}
	}()
	task(context.Background())
}

// Submit 提交一个任务。如果池已满，则阻塞直到有空位或池被关闭。
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

// SubmitWithTimeout 提交一个带超时的任务。
func (p *Pool) SubmitWithTimeout(task Task, timeout time.Duration) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return ErrPoolClosed
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case p.tasks <- task:
		return nil
	case <-timer.C:
		return ErrTaskTimeout
	case <-p.quit:
		return ErrPoolClosed
	}
}

// TrySubmit 尝试提交一个任务。如果池已满，立即返回 ErrPoolFull。
func (p *Pool) TrySubmit(task Task) error {
	if atomic.LoadInt32(&p.closed) == 1 {
		return ErrPoolClosed
	}

	select {
	case p.tasks <- task:
		return nil
	default:
		return ErrPoolFull
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
