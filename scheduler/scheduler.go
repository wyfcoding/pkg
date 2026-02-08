package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
	"github.com/wyfcoding/pkg/retry"
)

var (
	// ErrJobNameEmpty 任务名称为空。
	ErrJobNameEmpty = errors.New("job name is empty")
	// ErrJobIntervalInvalid 任务间隔非法。
	ErrJobIntervalInvalid = errors.New("job interval is invalid")
	// ErrJobAlreadyExists 任务名称重复。
	ErrJobAlreadyExists = errors.New("job already exists")
	// ErrJobHandlerNil 任务处理函数为空。
	ErrJobHandlerNil = errors.New("job handler is nil")
)

// Job 定义定时任务函数原型。
type Job func(ctx context.Context) error

// JobConfig 定义任务调度参数。
type JobConfig struct {
	Name            string        // 任务名称（唯一）。
	Interval        time.Duration // 调度间隔。
	Jitter          time.Duration // 抖动时间，用于打散同一时刻的任务触发。
	Timeout         time.Duration // 单次执行超时。
	RetryConfig     retry.Config  // 重试策略配置。
	RunOnStart      bool          // 是否在启动时立即执行一次。
	AllowConcurrent bool          // 是否允许任务并发执行。
	Enabled         bool          // 是否启用任务。
}

// Scheduler 负责任务的统一调度与生命周期管理。
type Scheduler struct {
	logger  *slog.Logger
	mu      sync.Mutex
	jobs    map[string]*jobRunner
	stop    chan struct{}
	wg      sync.WaitGroup
	metrics *schedulerMetrics
}

type jobRunner struct {
	cfg     JobConfig
	handler Job
	running int32
}

type schedulerMetrics struct {
	jobRuns     *prometheus.CounterVec
	jobDuration *prometheus.HistogramVec
	jobRunning  *prometheus.GaugeVec
}

// NewScheduler 创建任务调度器。
func NewScheduler(logger *logging.Logger) *Scheduler {
	return NewSchedulerWithMetrics(logger, nil)
}

// NewSchedulerWithMetrics 创建带指标采集的任务调度器。
func NewSchedulerWithMetrics(logger *logging.Logger, m *metrics.Metrics) *Scheduler {
	if logger == nil {
		logger = logging.Default()
	}

	var schedMetrics *schedulerMetrics
	if m != nil {
		schedMetrics = &schedulerMetrics{
			jobRuns: m.NewCounterVec(&prometheus.CounterOpts{
				Namespace: "pkg",
				Subsystem: "scheduler",
				Name:      "job_runs_total",
				Help:      "Total number of scheduled job runs",
			}, []string{"job", "status"}),
			jobDuration: m.NewHistogramVec(&prometheus.HistogramOpts{
				Namespace: "pkg",
				Subsystem: "scheduler",
				Name:      "job_duration_seconds",
				Help:      "Scheduled job execution duration",
				Buckets:   prometheus.DefBuckets,
			}, []string{"job", "status"}),
			jobRunning: m.NewGaugeVec(&prometheus.GaugeOpts{
				Namespace: "pkg",
				Subsystem: "scheduler",
				Name:      "job_running",
				Help:      "Current running jobs",
			}, []string{"job"}),
		}
	}

	return &Scheduler{
		logger:  logger.Logger,
		jobs:    make(map[string]*jobRunner),
		stop:    make(chan struct{}),
		metrics: schedMetrics,
	}
}

// AddJob 注册一个新的调度任务。
func (s *Scheduler) AddJob(cfg JobConfig, handler Job) error {
	if cfg.Name == "" {
		return ErrJobNameEmpty
	}
	if cfg.Interval <= 0 {
		return ErrJobIntervalInvalid
	}
	if handler == nil {
		return ErrJobHandlerNil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.jobs[cfg.Name]; exists {
		return ErrJobAlreadyExists
	}

	if cfg.RetryConfig == (retry.Config{}) {
		cfg.RetryConfig = retry.Config{MaxRetries: 0}
	}
	if cfg.Enabled == false {
		cfg.Enabled = true
	}

	s.jobs[cfg.Name] = &jobRunner{
		cfg:     cfg,
		handler: handler,
	}

	return nil
}

// Start 启动调度器并异步运行所有任务。
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, runner := range s.jobs {
		if !runner.cfg.Enabled {
			continue
		}
		s.wg.Add(1)
		go s.runJob(ctx, runner)
	}
}

// Stop 关闭调度器并等待所有任务退出。
func (s *Scheduler) Stop(ctx context.Context) error {
	close(s.stop)

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (s *Scheduler) runJob(ctx context.Context, runner *jobRunner) {
	defer s.wg.Done()

	if runner.cfg.RunOnStart {
		s.execute(ctx, runner)
	}

	ticker := time.NewTicker(runner.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if runner.cfg.Jitter > 0 {
				time.Sleep(randomJitter(runner.cfg.Jitter))
			}
			s.execute(ctx, runner)
		case <-s.stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) execute(ctx context.Context, runner *jobRunner) {
	if !runner.cfg.AllowConcurrent {
		if !atomic.CompareAndSwapInt32(&runner.running, 0, 1) {
			s.logger.Warn("scheduler job skipped (already running)", "job", runner.cfg.Name)
			if s.metrics != nil {
				s.metrics.jobRuns.WithLabelValues(runner.cfg.Name, "skipped").Inc()
			}
			return
		}
		defer atomic.StoreInt32(&runner.running, 0)
	}

	execCtx := ctx
	var cancel context.CancelFunc
	if runner.cfg.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, runner.cfg.Timeout)
		defer cancel()
	}

	start := time.Now()
	if s.metrics != nil {
		s.metrics.jobRunning.WithLabelValues(runner.cfg.Name).Inc()
	}
	err := retry.If(execCtx, func() error {
		return runner.handler(execCtx)
	}, func(err error) bool {
		return err != nil
	}, runner.cfg.RetryConfig)
	if s.metrics != nil {
		s.metrics.jobRunning.WithLabelValues(runner.cfg.Name).Dec()
	}

	if err != nil {
		if s.metrics != nil {
			s.metrics.jobRuns.WithLabelValues(runner.cfg.Name, "failed").Inc()
			s.metrics.jobDuration.WithLabelValues(runner.cfg.Name, "failed").Observe(time.Since(start).Seconds())
		}
		s.logger.Error("scheduler job failed", "job", runner.cfg.Name, "error", err)
		return
	}

	if s.metrics != nil {
		s.metrics.jobRuns.WithLabelValues(runner.cfg.Name, "success").Inc()
		s.metrics.jobDuration.WithLabelValues(runner.cfg.Name, "success").Observe(time.Since(start).Seconds())
	}
	s.logger.Debug("scheduler job succeeded", "job", runner.cfg.Name)
}

func randomJitter(maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}
	n := time.Now().UnixNano()
	if n < 0 {
		n = -n
	}
	return time.Duration(n % int64(maxJitter))
}
