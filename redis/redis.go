package redis

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/wyfcoding/pkg/config"
	"github.com/wyfcoding/pkg/logging"
)

// Client 是 redis.UniversalClient 的别名, 支持单节点, 哨兵, 集群.
type Client = redis.UniversalClient

var (
	redisOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "pkg",
			Subsystem: "redis",
			Name:      "ops_total",
			Help:      "The total number of redis operations",
		},
		[]string{"addr", "command", "status"},
	)
	redisDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "pkg",
			Subsystem: "redis",
			Name:      "duration_seconds",
			Help:      "The duration of redis operations",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"addr", "command"},
	)
)

const (
	pingTimeout = 5 * time.Second
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
		if err != nil && !errors.Is(err, redis.Nil) {
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
		if err != nil && !errors.Is(err, redis.Nil) {
			status = "error"
		}

		redisOps.WithLabelValues(h.addr, "pipeline", status).Inc()
		redisDuration.WithLabelValues(h.addr, "pipeline").Observe(duration)

		return err
	}
}

// NewClient 使用提供的配置创建一个新的 Redis UniversalClient (自动适配 Cluster/Sentinel/Standalone).
func NewClient(cfg *config.RedisConfig, logger *logging.Logger) (redis.UniversalClient, func(), error) {
	// 将配置转换为 UniversalOptions
	// 注意: config.RedisConfig 需要支持更多字段 (如 Addrs []string) 来完全支持集群.
	// 这里假设 cfg.Addr 可能包含逗号分隔的地址, 或者我们暂时只支持单节点但使用通用接口.
	// 更好的做法是让 config.RedisConfig 本身支持集群配置.
	// 这里做最小改动以支持 UniversalClient 接口.

	opts := &redis.UniversalOptions{
		Addrs:        []string{cfg.Addr}, // TODO: Support multiple addrs in config
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	client := redis.NewUniversalClient(opts)

	client.AddHook(&metricsHook{addr: cfg.Addr})

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Successfully connected to Redis", "addr", cfg.Addr)

	cleanup := func() {
		if err := client.Close(); err != nil {
			logger.Error("failed to close Redis client", "error", err)
		}
	}

	return client, cleanup, nil
}
