package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/robfig/cron/v3"
	"github.com/wyfcoding/pkg/lock"
	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
)

var (
	// ErrJobNameEmpty 任务名称为空。
	ErrJobNameEmpty = errors.New("job name is empty")
	// ErrJobHandlerNil 任务处理函数为空。
	ErrJobHandlerNil = errors.New("job handler is nil")
)

// JobFunc 定义单纯的任务函数。
type JobFunc func(ctx context.Context) error

// JobConfig 定义任务配置。
type JobConfig struct {
	Name            string               // 任务名称（唯一标识，用于锁和其他追踪）。
	CronExpr        string               // Cron 表达式 (例如 "* * * * *")。
	RunOnStart      bool                 // 是否启动时立即运行一次。
	Retry           int                  // 失败重试次数 (简单重试)。
	Timeout         time.Duration        // 单次执行超时。
	Lock            lock.DistributedLock // 分布式锁实例 (可选，若提供则启用分布式互斥)。
	LockTTL         time.Duration        // 锁的过期时间。
	AllowConcurrent bool                 // 允许并发 (单实例内并发，若有分布式锁则多实例也互斥)。
}

// Scheduler 分装 robfig/cron 的通用调度器。
type Scheduler struct {
	cron    *cron.Cron
	logger  *slog.Logger
	mu      sync.Mutex
	entries map[string]cron.EntryID
	jobs    map[string]*JobConfig
	metrics *schedulerMetrics
}

type schedulerMetrics struct {
	jobRuns     *prometheus.CounterVec
	jobDuration *prometheus.HistogramVec
	jobRunning  *prometheus.GaugeVec
}

// NewScheduler 创建新的调度器。
func NewScheduler(logger *logging.Logger, m *metrics.Metrics) *Scheduler {
	if logger == nil {
		logger = logging.Default()
	}

	cronLogger := &cronLoggerAdapter{logger: logger.Logger}
	c := cron.New(
		cron.WithSeconds(), // 支持秒级 cron (6位)
		cron.WithChain(cron.Recover(cronLogger), cron.SkipIfStillRunning(cronLogger)),
		cron.WithLogger(cronLogger),
	)

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
		cron:    c,
		logger:  logger.Logger,
		entries: make(map[string]cron.EntryID),
		jobs:    make(map[string]*JobConfig),
		metrics: schedMetrics,
	}
}

// AddJob 添加或更新任务。如果任务名已存在，会先移除旧任务。
func (s *Scheduler) AddJob(cfg JobConfig, handler JobFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cfg.Name == "" {
		return ErrJobNameEmpty
	}
	if handler == nil {
		return ErrJobHandlerNil
	}

	// 移除旧任务
	if id, exists := s.entries[cfg.Name]; exists {
		s.cron.Remove(id)
		delete(s.entries, cfg.Name)
	}

	wrapper := s.wrapJob(cfg, handler)

	// 添加到 cron
	id, err := s.cron.AddFunc(cfg.CronExpr, wrapper)
	if err != nil {
		return err
	}

	s.entries[cfg.Name] = id
	s.jobs[cfg.Name] = &cfg

	s.logger.Info("job added to scheduler", "name", cfg.Name, "cron", cfg.CronExpr)

	if cfg.RunOnStart {
		go wrapper()
	}

	return nil
}

// RemoveJob 移除任务。
func (s *Scheduler) RemoveJob(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, exists := s.entries[name]; exists {
		s.cron.Remove(id)
		delete(s.entries, name)
		delete(s.jobs, name)
		s.logger.Info("job removed from scheduler", "name", name)
	}
}

// Start 启动调度器。
func (s *Scheduler) Start(ctx context.Context) {
	s.cron.Start()
	s.logger.Info("scheduler started")
}

// Stop 停止调度器。
func (s *Scheduler) Stop(ctx context.Context) error {
	ctx = s.cron.Stop() // cron.Stop returns a context that is done when stop completes
	select {
	case <-ctx.Done():
		s.logger.Info("scheduler stopped")
		return nil
	case <-time.After(5 * time.Second): // Default timeout if context not provided or needed
		return errors.New("scheduler stop timed out")
	}
}

// wrapJob 包装任务执行逻辑（锁、重试、指标、超时）。
func (s *Scheduler) wrapJob(cfg JobConfig, handler JobFunc) func() {
	return func() {
		ctx := context.Background()
		// 超时控制
		if cfg.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
			defer cancel()
		}

		start := time.Now()

		// 分布式锁
		if cfg.Lock != nil {
			// 尝试加锁 (非阻塞，失败即跳过)
			lockKey := "scheduler:lock:" + cfg.Name
			ttl := cfg.LockTTL
			if ttl == 0 {
				ttl = 10 * time.Second // 默认 10s
			}
			// 使用 TryLock，waitTimeout=0 (立即返回)
			token, err := cfg.Lock.TryLock(ctx, lockKey, ttl, 0)
			if err != nil {
				s.logger.Debug("job skipped (locked by other instance)", "name", cfg.Name)
				if s.metrics != nil {
					s.metrics.jobRuns.WithLabelValues(cfg.Name, "skipped_locked").Inc()
				}
				return
			}
			defer func() {
				// 使用新的 context 释放锁，防止 ctx 已超时
				unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if err := cfg.Lock.Unlock(unlockCtx, lockKey, token); err != nil {
					s.logger.Warn("failed to unlock job", "name", cfg.Name, "error", err)
				}
			}()
		}

		if s.metrics != nil {
			s.metrics.jobRunning.WithLabelValues(cfg.Name).Inc()
			defer s.metrics.jobRunning.WithLabelValues(cfg.Name).Dec()
		}

		var err error
		// 执行逻辑 (含简单重试)
		for i := 0; i <= cfg.Retry; i++ {
			err = handler(ctx)
			if err == nil {
				break
			}
			if i < cfg.Retry {
				time.Sleep(time.Duration(i+1) * 100 * time.Millisecond) // 线性退避
				s.logger.Warn("job failed, retrying", "name", cfg.Name, "attempt", i+1, "error", err)
			}
		}

		duration := time.Since(start).Seconds()

		if err != nil {
			s.logger.Error("job execution failed", "name", cfg.Name, "error", err)
			if s.metrics != nil {
				s.metrics.jobRuns.WithLabelValues(cfg.Name, "failed").Inc()
				s.metrics.jobDuration.WithLabelValues(cfg.Name, "failed").Observe(duration)
			}
		} else {
			s.logger.Info("job execution success", "name", cfg.Name, "duration", duration)
			if s.metrics != nil {
				s.metrics.jobRuns.WithLabelValues(cfg.Name, "success").Inc()
				s.metrics.jobDuration.WithLabelValues(cfg.Name, "success").Observe(duration)
			}
		}
	}
}

// cronLoggerAdapter 适配 cron.Logger 接口
type cronLoggerAdapter struct {
	logger *slog.Logger
}

func (l *cronLoggerAdapter) Info(msg string, keysAndValues ...any) {
	l.logger.Info(msg, keysAndValues...)
}

func (l *cronLoggerAdapter) Error(err error, msg string, keysAndValues ...any) {
	l.logger.Error(msg, append(keysAndValues, "error", err)...)
}
