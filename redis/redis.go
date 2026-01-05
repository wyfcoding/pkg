package redis

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// Client 是 redis.Client 的别名，方便业务层直接使用而无需导入原生包
type Client = redis.Client

var (
	redisOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_ops_total",
			Help: "The total number of redis operations",
		},
		[]string{"addr", "command", "status"},
	)
	redisDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redis_duration_seconds",
			Help:    "The duration of redis operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"addr", "command"},
	)
)

func init() {
	prometheus.MustRegister(redisOps, redisDuration)
}

type metricsHook struct {
	addr string
}

func (h *metricsHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h *metricsHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmd)
		duration := time.Since(start).Seconds()

		status := "success"
		if err != nil && err != redis.Nil {
			status = "error"
		}

		redisOps.WithLabelValues(h.addr, cmd.Name(), status).Inc()
		redisDuration.WithLabelValues(h.addr, cmd.Name()).Observe(duration)

		return err
	}
}

func (h *metricsHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmds)
		duration := time.Since(start).Seconds()

		status := "success"
		if err != nil && err != redis.Nil {
			status = "error"
		}

		redisOps.WithLabelValues(h.addr, "pipeline", status).Inc()
		redisDuration.WithLabelValues(h.addr, "pipeline").Observe(duration)

		return err
	}
}

// NewClient 使用提供的配置创建一个新的 Redis 客户端。
// 返回一个 *redis.Client 实例、清理函数和连接失败时的错误。
func NewClient(cfg *config.RedisConfig, logger *logging.Logger) (*redis.Client, func(), error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// Add metrics hook
	client.AddHook(&metricsHook{addr: cfg.Addr})

	// 创建一个带超时机制的上下文，用于Ping操作。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel() // 确保上下文在函数退出时被取消。

	// 尝试Ping Redis服务器以验证连接的可用性。
	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Successfully connected to Redis", "addr", client.Options().Addr)

	// 定义一个清理函数，用于在应用程序关闭时优雅地关闭Redis客户端。
	cleanup := func() {
		if err := client.Close(); err != nil {
			logger.Error("failed to close Redis client", "error", err)
		}
	}

	return client, cleanup, nil
}
