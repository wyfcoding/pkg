// Package config 提供了统一的配置加载与管理能力，支持环境变量覆盖、实时验证及热加载。
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"gorm.io/gorm/logger"
)

// Config 全局顶级配置结构，汇集了所有模块的配置信息。
type Config struct {
	Version        string               `mapstructure:"version" toml:"version"`               // 配置文件版本。
	Server         ServerConfig         `mapstructure:"server" toml:"server"`                 // 服务器基础配置（HTTP/gRPC）。
	Data           DataConfig           `mapstructure:"data" toml:"data"`                     // 数据源配置（DB/Redis/MQ 等）。
	Log            LogConfig            `mapstructure:"log" toml:"log"`                       // 日志记录配置。
	JWT            JWTConfig            `mapstructure:"jwt" toml:"jwt"`                       // JWT 身份认证配置。
	Snowflake      SnowflakeConfig      `mapstructure:"snowflake" toml:"snowflake"`           // 雪花算法 ID 生成器配置。
	MessageQueue   MessageQueueConfig   `mapstructure:"messagequeue" toml:"messagequeue"`     // 消息队列配置（Kafka）。
	Minio          MinioConfig          `mapstructure:"minio" toml:"minio"`                   // 对象存储 MinIO 配置。
	Hadoop         HadoopConfig         `mapstructure:"hadoop" toml:"hadoop"`                 // Hadoop/HDFS 相关配置。
	Tracing        TracingConfig        `mapstructure:"tracing" toml:"tracing"`               // 分布式链路追踪配置。
	Metrics        MetricsConfig        `mapstructure:"metrics" toml:"metrics"`               // 监控指标暴露配置。
	RateLimit      RateLimitConfig      `mapstructure:"ratelimit" toml:"ratelimit"`           // 全局限流策略配置。
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuitbreaker" toml:"circuitbreaker"` // 熔断器通用配置。
	Cache          CacheConfig          `mapstructure:"cache" toml:"cache"`                   // 本地或通用缓存配置。
	Lock           LockConfig           `mapstructure:"lock" toml:"lock"`                     // 分布式锁通用配置。
	Services       ServicesConfig       `mapstructure:"services" toml:"services"`             // 下游依赖微服务地址映射。
}

// ServerConfig 定义服务器运行时的基础网络与环境参数。
type ServerConfig struct {
	Name        string `mapstructure:"name" toml:"name" validate:"required"`                          // 服务唯一标识名称。
	Environment string `mapstructure:"environment" toml:"environment" validate:"oneof=dev test prod"` // 运行环境（开发/测试/生产）。
	HTTP        struct {
		Addr         string        `mapstructure:"addr" toml:"addr"`                                     // HTTP 监听地址。
		Port         int           `mapstructure:"port" toml:"port" validate:"required,min=1,max=65535"` // HTTP 监听端口。
		Timeout      time.Duration `mapstructure:"timeout" toml:"timeout"`                               // 全局读写超时。
		ReadTimeout  time.Duration `mapstructure:"read_timeout" toml:"read_timeout"`                     // 读取请求超时。
		WriteTimeout time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`                   // 写入响应超时。
		IdleTimeout  time.Duration `mapstructure:"idle_timeout" toml:"idle_timeout"`                     // 空闲连接保持超时。
	} `mapstructure:"http" toml:"http"`
	GRPC struct {
		Addr                 string        `mapstructure:"addr" toml:"addr"`                                     // gRPC 监听地址。
		Port                 int           `mapstructure:"port" toml:"port" validate:"required,min=1,max=65535"` // gRPC 监听端口。
		Timeout              time.Duration `mapstructure:"timeout" toml:"timeout"`                               // 请求处理超时。
		MaxRecvMsgSize       int           `mapstructure:"max_recv_msg_size" toml:"max_recv_msg_size"`           // 最大接收消息字节数。
		MaxSendMsgSize       int           `mapstructure:"max_send_msg_size" toml:"max_send_msg_size"`           // 最大发送消息字节数。
		MaxConcurrentStreams int           `mapstructure:"max_concurrent_streams" toml:"max_concurrent_streams"` // 最大并发流处理数。
	} `mapstructure:"grpc" toml:"grpc"`
}

// DataConfig 汇集了所有持久化存储与中间件的数据源配置。
type DataConfig struct {
	Database      DatabaseConfig      `mapstructure:"database" toml:"database"`           // 默认关系型数据库配置。
	Shards        []DatabaseConfig    `mapstructure:"shards" toml:"shards"`               // 数据库分片集群配置。
	Redis         RedisConfig         `mapstructure:"redis" toml:"redis"`                 // Redis 缓存/存储配置。
	BigCache      BigCacheConfig      `mapstructure:"bigcache" toml:"bigcache"`           // 本地内存缓存配置。
	MongoDB       MongoDBConfig       `mapstructure:"mongodb" toml:"mongodb"`             // MongoDB 文档数据库配置。
	ClickHouse    ClickHouseConfig    `mapstructure:"clickhouse" toml:"clickhouse"`       // ClickHouse 分析型数据库配置。
	Neo4j         Neo4jConfig         `mapstructure:"neo4j" toml:"neo4j"`                 // Neo4j 图数据库配置。
	Elasticsearch ElasticsearchConfig `mapstructure:"elasticsearch" toml:"elasticsearch"` // Elasticsearch 搜索引擎配置。
}

// DatabaseConfig 定义单数据库实例连接与连接池参数。
type DatabaseConfig struct {
	Driver          string          `mapstructure:"driver" toml:"driver" validate:"required"`   // 驱动类型（mysql/postgres/clickhouse）。
	DSN             string          `mapstructure:"dsn" toml:"dsn" validate:"required"`         // 数据库连接字符串 (Data Source Name)。
	MaxIdleConns    int             `mapstructure:"max_idle_conns" toml:"max_idle_conns"`       // 连接池最大空闲连接数。
	MaxOpenConns    int             `mapstructure:"max_open_conns" toml:"max_open_conns"`       // 连接池最大打开连接数。
	ConnMaxLifetime time.Duration   `mapstructure:"conn_max_lifetime" toml:"conn_max_lifetime"` // 连接可重用的最长时间。
	LogLevel        logger.LogLevel `mapstructure:"log_level" toml:"log_level"`                 // GORM 日志级别。
	SlowThreshold   time.Duration   `mapstructure:"slow_threshold" toml:"slow_threshold"`       // 慢 SQL 判定阈值。
}

// RedisConfig 定义 Redis 连接与池化参数。
type RedisConfig struct {
	Addr         string        `mapstructure:"addr" toml:"addr" validate:"required"` // Redis 服务器地址（IP:Port）。
	Password     string        `mapstructure:"password" toml:"password"`             // 认证密码。
	DB           int           `mapstructure:"db" toml:"db"`                         // 指定使用的数据库索引 (0-15)。
	ReadTimeout  time.Duration `mapstructure:"read_timeout" toml:"read_timeout"`     // 读取超时时间。
	WriteTimeout time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`   // 写入超时时间。
	PoolSize     int           `mapstructure:"pool_size" toml:"pool_size"`           // 连接池最大连接数。
	MinIdleConns int           `mapstructure:"min_idle_conns" toml:"min_idle_conns"` // 连接池维护的最小空闲连接数。
}

// LogConfig 定义日志输出、级别与切割策略。
type LogConfig struct {
	Level      string `mapstructure:"level" toml:"level"`             // 日志级别 (debug/info/warn/error)。
	Format     string `mapstructure:"format" toml:"format"`           // 输出格式 (json/text)。
	Output     string `mapstructure:"output" toml:"output"`           // 输出目标 (stdout/file)。
	File       string `mapstructure:"file" toml:"file"`               // 日志文件路径。
	MaxSize    int    `mapstructure:"max_size" toml:"max_size"`       // 每个文件最大尺寸 (MB)。
	MaxBackups int    `mapstructure:"max_backups" toml:"max_backups"` // 保留的旧日志文件最大个数。
	MaxAge     int    `mapstructure:"max_age" toml:"max_age"`         // 保留的旧日志文件最大天数。
	Compress   bool   `mapstructure:"compress" toml:"compress"`       // 是否压缩旧日志。
}

// JWTConfig 身份认证令牌相关配置。
type JWTConfig struct {
	Secret         string        `mapstructure:"secret" toml:"secret"`                   // 签名密钥。
	Issuer         string        `mapstructure:"issuer" toml:"issuer"`                   // 签发者名称。
	ExpireDuration time.Duration `mapstructure:"expire_duration" toml:"expire_duration"` // 令牌有效期。
}

// SnowflakeConfig 雪花算法分布式 ID 生成器参数。
type SnowflakeConfig struct {
	Type      string `mapstructure:"type" toml:"type"`             // 算法实现类型。
	StartTime string `mapstructure:"start_time" toml:"start_time"` // 算法起始基准时间 (YYYY-MM-DD)。
	MachineID int64  `mapstructure:"machine_id" toml:"machine_id"` // 集群内唯一节点 ID。
}

// MessageQueueConfig 聚合所有消息中间件配置。
type MessageQueueConfig struct {
	Kafka KafkaConfig `mapstructure:"kafka" toml:"kafka"` // Kafka 具体配置。
}

// KafkaConfig 定义 Kafka 生产者与消费者参数。
type KafkaConfig struct {
	Brokers        []string      `mapstructure:"brokers" toml:"brokers"`                 // Broker 地址列表。
	Topic          string        `mapstructure:"topic" toml:"topic"`                     // 默认主题名称。
	GroupID        string        `mapstructure:"group_id" toml:"group_id"`               // 消费者组 ID。
	DialTimeout    time.Duration `mapstructure:"dial_timeout" toml:"dial_timeout"`       // 连接建立超时。
	ReadTimeout    time.Duration `mapstructure:"read_timeout" toml:"read_timeout"`       // 读取消息超时。
	WriteTimeout   time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`     // 写入消息超时。
	MinBytes       int           `mapstructure:"min_bytes" toml:"min_bytes"`             // 消费者最小抓取字节。
	MaxBytes       int           `mapstructure:"max_bytes" toml:"max_bytes"`             // 消费者最大抓取字节。
	MaxWait        time.Duration `mapstructure:"max_wait" toml:"max_wait"`               // 消费者等待新消息的最长时间。
	MaxAttempts    int           `mapstructure:"max_attempts" toml:"max_attempts"`       // 消息发送最大尝试次数。
	CommitInterval time.Duration `mapstructure:"commit_interval" toml:"commit_interval"` // 消费者 Offset 提交间隔。
	RequiredAcks   int           `mapstructure:"required_acks" toml:"required_acks"`     // 写入确认级别 (-1, 0, 1)。
	Async          bool          `mapstructure:"async" toml:"async"`                     // 是否启用异步发送。
}

// MinioConfig 定义 S3 兼容对象存储 MinIO 的连接参数。
type MinioConfig struct {
	Endpoint        string `mapstructure:"endpoint" toml:"endpoint"`                   // 服务接入地址。
	AccessKeyID     string `mapstructure:"access_key_id" toml:"access_key_id"`         // 访问 ID。
	SecretAccessKey string `mapstructure:"secret_access_key" toml:"secret_access_key"` // 访问密钥。
	UseSSL          bool   `mapstructure:"use_ssl" toml:"use_ssl"`                     // 是否使用加密传输。
	BucketName      string `mapstructure:"bucket_name" toml:"bucket_name"`             // 默认存储桶名称。
}

// TracingConfig 分布式链路追踪（OpenTelemetry/Jaeger）配置。
type TracingConfig struct {
	Enabled      bool   `mapstructure:"enabled" toml:"enabled"`             // 是否启用追踪。
	ServiceName  string `mapstructure:"service_name" toml:"service_name"`   // 注册到追踪系统的服务名。
	OTLPEndpoint string `mapstructure:"otlp_endpoint" toml:"otlp_endpoint"` // OTLP 接收器地址。
}

// MetricsConfig 普罗米修斯监控指标暴露配置。
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled" toml:"enabled"` // 是否启用指标采集。
	Port    string `mapstructure:"port" toml:"port"`       // 指标暴露监听端口。
	Path    string `mapstructure:"path" toml:"path"`       // 指标拉取路径（默认 /metrics）。
}

// RateLimitConfig 定义令牌桶或漏桶限流参数。
type RateLimitConfig struct {
	Enabled bool `mapstructure:"enabled" toml:"enabled"` // 是否启用全局限流。
	Rate    int  `mapstructure:"rate" toml:"rate"`       // 每秒允许的请求数 (QPS)。
	Burst   int  `mapstructure:"burst" toml:"burst"`     // 允许的最大突发请求。
}

// CircuitBreakerConfig 定义熔断器（如 Gobreaker）的保护策略。
type CircuitBreakerConfig struct {
	Enabled     bool          `mapstructure:"enabled" toml:"enabled"`           // 是否启用熔断。
	MaxRequests uint32        `mapstructure:"max_requests" toml:"max_requests"` // 半开状态允许的最大请求。
	Interval    time.Duration `mapstructure:"interval" toml:"interval"`         // 周期性重置计数器的时间间隔。
	Timeout     time.Duration `mapstructure:"timeout" toml:"timeout"`           // 熔断开启后的冷却恢复时间。
}

// CacheConfig 通用缓存策略配置。
type CacheConfig struct {
	Prefix            string        `mapstructure:"prefix" toml:"prefix"`                         // 缓存 Key 统一前缀。
	DefaultExpiration time.Duration `mapstructure:"default_expiration" toml:"default_expiration"` // 默认过期时间。
	CleanupInterval   time.Duration `mapstructure:"cleanup_interval" toml:"cleanup_interval"`     // 自动清理周期。
}

// LockConfig 分布式锁通用配置。
type LockConfig struct {
	Prefix            string        `mapstructure:"prefix" toml:"prefix"`                         // 锁 Key 统一前缀。
	DefaultExpiration time.Duration `mapstructure:"default_expiration" toml:"default_expiration"` // 锁的默认持有时间（防死锁）。
	MaxRetries        int           `mapstructure:"max_retries" toml:"max_retries"`               // 获取锁的最大重试次数。
	RetryDelay        time.Duration `mapstructure:"retry_delay" toml:"retry_delay"`               // 每次重试的等待延迟。
}

// ServicesConfig 下游依赖微服务的地址注册映射。
type ServicesConfig map[string]ServiceAddr

// ServiceAddr 定义单个微服务的接入地址。
type ServiceAddr struct {
	GRPCAddr string `mapstructure:"grpc_addr" toml:"grpc_addr"` // gRPC 内部访问地址 (Host:Port)。
	HTTPAddr string `mapstructure:"http_addr" toml:"http_addr"` // HTTP 外部/内部访问地址。
}

// HadoopConfig 大数据生态组件 HDFS 配置。
type HadoopConfig struct {
	NameNode string `mapstructure:"name_node" toml:"name_node"` // NameNode 管理地址。
}

// BigCacheConfig 高性能本地内存缓存参数。
type BigCacheConfig struct {
	Shards             int           `mapstructure:"shards" toml:"shards"`                               // 内存分片数（减少锁竞争）。
	LifeWindow         time.Duration `mapstructure:"life_window" toml:"life_window"`                     // 记录的生命周期。
	CleanWindow        time.Duration `mapstructure:"clean_window" toml:"clean_window"`                   // 自动清理无效条目的时间间隔。
	MaxEntrySize       int           `mapstructure:"max_entry_size" toml:"max_entry_size"`               // 每条目最大字节数。
	Verbose            bool          `mapstructure:"verbose" toml:"verbose"`                             // 是否开启详细模式。
	HardMaxCacheSize   int           `mapstructure:"hard_max_cache_size" toml:"hard_max_cache_size"`     // 内存占用的硬件软上限 (MB)。
	OnRemoveWithReason bool          `mapstructure:"on_remove_with_reason" toml:"on_remove_with_reason"` // 移除条目时是否记录原因。
}

// MongoDBConfig 定义 MongoDB 的连接参数。
type MongoDBConfig struct {
	URI      string `mapstructure:"uri" toml:"uri"`           // 连接字符串。
	Database string `mapstructure:"database" toml:"database"` // 默认数据库名称。
}

// ClickHouseConfig 定义 ClickHouse 列式存储的连接参数。
type ClickHouseConfig struct {
	Addr     string `mapstructure:"addr" toml:"addr"`         // 服务器地址 (Host:Port)。
	Database string `mapstructure:"database" toml:"database"` // 默认数据库。
	Username string `mapstructure:"username" toml:"username"` // 登录用户。
	Password string `mapstructure:"password" toml:"password"` // 登录密码。
}

// Neo4jConfig 定义 Neo4j 图数据库的连接参数。
type Neo4jConfig struct {
	URI      string `mapstructure:"uri" toml:"uri"`           // 接入地址 (bolt:// 或 neo4j://)。
	Username string `mapstructure:"username" toml:"username"` // 用户。
	Password string `mapstructure:"password" toml:"password"` // 密码。
}

// ElasticsearchConfig 定义搜索引擎集群连接参数。
type ElasticsearchConfig struct {
	Addresses []string `mapstructure:"addresses" toml:"addresses"` // 节点列表。
	Username  string   `mapstructure:"username" toml:"username"`   // 访问用户。
	Password  string   `mapstructure:"password" toml:"password"`   // 访问密码。
}

var vInstance = viper.New()

// Load 全生产级的配置加载逻辑。
func Load(path string, conf any) error {
	vInstance.SetConfigFile(path)
	vInstance.SetConfigType("toml")

	// 1. 设置环境变量支持。
	vInstance.SetEnvPrefix("APP") // 环境变量前缀 APP。
	vInstance.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	vInstance.AutomaticEnv()

	// 2. 加载基础文件。
	if err := vInstance.ReadInConfig(); err != nil {
		return fmt.Errorf("read config error: %w", err)
	}

	// 3. 初始解析。
	if err := vInstance.Unmarshal(conf); err != nil {
		return fmt.Errorf("unmarshal config error: %w", err)
	}

	// 4. 实时校验。
	validate := validator.New()
	if err := validate.Struct(conf); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// 5. 开启热加载 (带防抖逻辑)。
	vInstance.WatchConfig()
	vInstance.OnConfigChange(func(e fsnotify.Event) {
		slog.Info("detecting config change", "file", e.Name)
		// 延迟 500ms 确保文件写操作已完成 (针对某些编辑器的原子保存行为)。
		const debounceTimeout = 500 * time.Millisecond
		time.Sleep(debounceTimeout)

		if unmarshalErr := vInstance.Unmarshal(conf); unmarshalErr != nil {
			slog.Error("reload config unmarshal failed", "error", unmarshalErr)

			return
		}

		if validateErr := validate.Struct(conf); validateErr != nil {
			slog.Error("reload config validation failed", "error", validateErr)
		} else {
			slog.Info("config hot-reloaded and validated successfully")
		}
	})

	return nil
}

// PrintWithMask 脱敏打印当前配置，方便生产环境排查。
func PrintWithMask(conf any) {
	data, err := json.Marshal(conf)
	if err != nil {
		slog.Error("failed to marshal config for printing", "error", err)

		return
	}

	var m map[string]any
	if unmarshalErr := json.Unmarshal(data, &m); unmarshalErr != nil {
		slog.Error("failed to unmarshal config for masking", "error", unmarshalErr)

		return
	}

	// 简单的脱敏递归函数。
	mask(m)

	maskedJSON, marshalErr := json.MarshalIndent(m, "  ", "  ")
	if marshalErr != nil {
		slog.Error("failed to marshal masked config", "error", marshalErr)

		return
	}

	slog.Info("Current effective configuration", "config", string(maskedJSON))
}

func mask(m map[string]any) {
	sensitiveKeys := []string{"password", "secret", "dsn", "key", "token"}

	for key, val := range m {
		// 递归处理子 Map。
		if subMap, ok := val.(map[string]any); ok {
			mask(subMap)

			continue
		}

		// 递归处理数组中的 Map。
		if slice, ok := val.([]any); ok {
			for _, item := range slice {
				if itemMap, ok := item.(map[string]any); ok {
					mask(itemMap)
				}
			}

			continue
		}

		// 执行脱敏。
		for _, sensitiveKey := range sensitiveKeys {
			if strings.Contains(strings.ToLower(key), sensitiveKey) {
				m[key] = "******"

				break
			}
		}
	}
}

// GetViper 返回底层的 Viper 实例，供需要直接读取动态值的场景使用。
func GetViper() *viper.Viper {
	return vInstance
}