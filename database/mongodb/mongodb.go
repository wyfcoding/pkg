package mongodb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"          // MongoDB Go驱动。
	"go.mongodb.org/mongo-driver/mongo/options"  // MongoDB客户端选项。
	"go.mongodb.org/mongo-driver/mongo/readpref" // MongoDB读偏好设置。
)

var (
	mongoOps = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mongo_ops_total",
			Help: "The total number of mongo operations",
		},
		[]string{"command", "status"},
	)
	mongoDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mongo_duration_seconds",
			Help:    "The duration of mongo operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"command"},
	)
)

func init() {
	prometheus.MustRegister(mongoOps, mongoDuration)
}

// Config 封装了 MongoDB 驱动初始化所需的各类参数。
type Config struct { // MongoDB 配置结构，已对齐。
	URI            string        `toml:"uri"`             // 连接字符串 (例如: mongodb://user:pass@host:27017)。
	Database       string        `toml:"database"`        // 默认操作的数据库名称。
	ConnectTimeout time.Duration `toml:"connect_timeout"` // 连接建立超时阈值。
	MinPoolSize    uint64        `toml:"min_pool_size"`   // 最小空闲连接池规模。
	MaxPoolSize    uint64        `toml:"max_pool_size"`   // 最大并发连接数限制。
}

// NewMongoClient 初始化并返回一个功能增强的 MongoDB 客户端及其对应的清理闭包。
// 架构设计：利用 CommandMonitor 自动采集每一次查询、写入的延迟与成功率。
func NewMongoClient(conf *Config) (*mongo.Client, func(), error) {
	// 创建一个带超时机制的上下文，用于连接操作。
	ctx, cancel := context.WithTimeout(context.Background(), conf.ConnectTimeout)
	defer cancel() // 确保上下文在函数退出时被取消。

	// 配置指标监控。
	monitor := &event.CommandMonitor{
		Succeeded: func(_ context.Context, evt *event.CommandSucceededEvent) {
			mongoOps.WithLabelValues(evt.CommandName, "success").Inc()
			mongoDuration.WithLabelValues(evt.CommandName).Observe(evt.Duration.Seconds())
		},
		Failed: func(_ context.Context, evt *event.CommandFailedEvent) {
			mongoOps.WithLabelValues(evt.CommandName, "failed").Inc()
			mongoDuration.WithLabelValues(evt.CommandName).Observe(evt.Duration.Seconds())
		},
	}

	clientOpts := options.Client().
		ApplyURI(conf.URI).
		SetMinPoolSize(conf.MinPoolSize).
		SetMaxPoolSize(conf.MaxPoolSize).
		SetMonitor(monitor)

	// 使用配置的URI、最小和最大连接池大小来创建MongoDB客户端。
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to mongodb: %w", err)
	}

	// 尝试Ping MongoDB的主节点以验证连接的可用性。
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, nil, fmt.Errorf("failed to ping mongodb: %w", err)
	}

	slog.Info("mongodb client initialized successfully", "db", conf.Database)

	// 定义一个清理函数，用于在应用程序关闭时优雅地断开MongoDB连接。
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // 设定断开连接的超时时间。
		defer cancel()                                                           // 确保上下文在函数退出时被取消。
		if err := client.Disconnect(ctx); err != nil {
			slog.Error("failed to disconnect from mongodb", "error", err)
		}
	}

	return client, cleanup, nil
}
