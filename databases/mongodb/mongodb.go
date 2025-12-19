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

// Config 结构体用于 MongoDB 数据库配置。
// 它包含了连接MongoDB所需的所有参数。
// 注意：此处的Config结构体与 pkg/config/config.go 中的 MongoDBConfig 结构体功能相似，
// 但在不同的包中独立定义。
type Config struct {
	URI            string        `toml:"uri"`             // MongoDB的连接URI，例如 "mongodb://username:password@host:port/database"。
	Database       string        `toml:"database"`        // 默认连接的数据库名称。
	ConnectTimeout time.Duration `toml:"connect_timeout"` // 连接超时时间。
	MinPoolSize    uint64        `toml:"min_pool_size"`   // 连接池中维护的最小连接数。
	MaxPoolSize    uint64        `toml:"max_pool_size"`   // 连接池中维护的最大连接数。
}

// NewMongoClient 创建一个新的 MongoDB 客户端连接。
// 它根据传入的配置建立连接，并返回 `*mongo.Client` 实例和一个用于关闭连接的清理函数。
func NewMongoClient(conf *Config) (*mongo.Client, func(), error) {
	// 创建一个带超时机制的上下文，用于连接操作。
	ctx, cancel := context.WithTimeout(context.Background(), conf.ConnectTimeout)
	defer cancel() // 确保上下文在函数退出时被取消。

	// Configure metrics monitor
	monitor := &event.CommandMonitor{
		Succeeded: func(ctx context.Context, evt *event.CommandSucceededEvent) {
			mongoOps.WithLabelValues(evt.CommandName, "success").Inc()
			mongoDuration.WithLabelValues(evt.CommandName).Observe(evt.Duration.Seconds())
		},
		Failed: func(ctx context.Context, evt *event.CommandFailedEvent) {
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
	ctx, cancel = context.WithTimeout(context.Background(), conf.ConnectTimeout)
	defer cancel() // 确保上下文在函数退出时被取消。
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, nil, fmt.Errorf("failed to ping mongodb: %w", err)
	}

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
