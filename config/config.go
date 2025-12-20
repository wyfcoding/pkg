// Package config 提供了应用程序的配置管理功能。
// 它定义了所有服务相关的配置结构体，支持从TOML文件加载配置，并通过环境变量进行覆盖，
// 同时提供了配置热加载的能力。
package config

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify" // 文件系统事件通知库
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper" // 强大的配置管理库
	"gorm.io/gorm/logger"
)

// Config 是应用程序的顶级配置结构体。
// 它包含了所有子模块和服务的配置信息。
type Config struct {
	Version        string               `mapstructure:"version" toml:"version"`
	Server         ServerConfig         `mapstructure:"server" toml:"server"`
	Data           DataConfig           `mapstructure:"data" toml:"data"`
	Log            LogConfig            `mapstructure:"log" toml:"log"`
	JWT            JWTConfig            `mapstructure:"jwt" toml:"jwt"`
	Snowflake      SnowflakeConfig      `mapstructure:"snowflake" toml:"snowflake"`
	MessageQueue   MessageQueueConfig   `mapstructure:"messagequeue" toml:"messagequeue"`
	Minio          MinioConfig          `mapstructure:"minio" toml:"minio"`
	Hadoop         HadoopConfig         `mapstructure:"hadoop" toml:"hadoop"`
	Tracing        TracingConfig        `mapstructure:"tracing" toml:"tracing"`
	Metrics        MetricsConfig        `mapstructure:"metrics" toml:"metrics"`
	RateLimit      RateLimitConfig      `mapstructure:"ratelimit" toml:"ratelimit"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuitbreaker" toml:"circuitbreaker"`
	Cache          CacheConfig          `mapstructure:"cache" toml:"cache"`
	Lock           LockConfig           `mapstructure:"lock" toml:"lock"`
	Services       ServicesConfig       `mapstructure:"services" toml:"services"`
}

// ServerConfig 定义服务器配置。
type ServerConfig struct {
	Name        string `mapstructure:"name" toml:"name" validate:"required"`
	Environment string `mapstructure:"environment" toml:"environment" validate:"oneof=dev test prod"`
	HTTP        struct {
		Addr         string        `mapstructure:"addr" toml:"addr"`
		Port         int           `mapstructure:"port" toml:"port" validate:"required,min=1,max=65535"`
		Timeout      time.Duration `mapstructure:"timeout" toml:"timeout"`
		ReadTimeout  time.Duration `mapstructure:"read_timeout" toml:"read_timeout"`
		WriteTimeout time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`
		IdleTimeout  time.Duration `mapstructure:"idle_timeout" toml:"idle_timeout"`
	} `mapstructure:"http" toml:"http"`
	GRPC struct {
		Addr                 string        `mapstructure:"addr" toml:"addr"`
		Port                 int           `mapstructure:"port" toml:"port" validate:"required,min=1,max=65535"`
		Timeout              time.Duration `mapstructure:"timeout" toml:"timeout"`
		MaxRecvMsgSize       int           `mapstructure:"max_recv_msg_size" toml:"max_recv_msg_size"`
		MaxSendMsgSize       int           `mapstructure:"max_send_msg_size" toml:"max_send_msg_size"`
		MaxConcurrentStreams int           `mapstructure:"max_concurrent_streams" toml:"max_concurrent_streams"`
	} `mapstructure:"grpc" toml:"grpc"`
}

// DataConfig 定义数据相关的配置。
type DataConfig struct {
	Database      DatabaseConfig      `mapstructure:"database" toml:"database"`
	Shards        []DatabaseConfig    `mapstructure:"shards" toml:"shards"`
	Redis         RedisConfig         `mapstructure:"redis" toml:"redis"`
	BigCache      BigCacheConfig      `mapstructure:"bigcache" toml:"bigcache"`
	MongoDB       MongoDBConfig       `mapstructure:"mongodb" toml:"mongodb"`
	ClickHouse    ClickHouseConfig    `mapstructure:"clickhouse" toml:"clickhouse"`
	Neo4j         Neo4jConfig         `mapstructure:"neo4j" toml:"neo4j"`
	Elasticsearch ElasticsearchConfig `mapstructure:"elasticsearch" toml:"elasticsearch"`
}

// DatabaseConfig 定义数据库（MySQL）配置。
type DatabaseConfig struct {
	Driver          string          `mapstructure:"driver" toml:"driver" validate:"required"`
	DSN             string          `mapstructure:"dsn" toml:"dsn" validate:"required"`
	MaxIdleConns    int             `mapstructure:"max_idle_conns" toml:"max_idle_conns"`
	MaxOpenConns    int             `mapstructure:"max_open_conns" toml:"max_open_conns"`
	ConnMaxLifetime time.Duration   `mapstructure:"conn_max_lifetime" toml:"conn_max_lifetime"`
	LogLevel        logger.LogLevel `mapstructure:"log_level" toml:"log_level"`
	SlowThreshold   time.Duration   `mapstructure:"slow_threshold" toml:"slow_threshold"`
}

// RedisConfig 定义 Redis 配置。
type RedisConfig struct {
	Addr         string        `mapstructure:"addr" toml:"addr" validate:"required"`
	Password     string        `mapstructure:"password" toml:"password"`
	DB           int           `mapstructure:"db" toml:"db"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" toml:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`
	PoolSize     int           `mapstructure:"pool_size" toml:"pool_size"`
	MinIdleConns int           `mapstructure:"min_idle_conns" toml:"min_idle_conns"`
}

// LogConfig 定义日志配置。
type LogConfig struct {
	Level      string `mapstructure:"level" toml:"level"`
	Format     string `mapstructure:"format" toml:"format"`
	Output     string `mapstructure:"output" toml:"output"`
	File       string `mapstructure:"file" toml:"file"`
	MaxSize    int    `mapstructure:"max_size" toml:"max_size"`
	MaxBackups int    `mapstructure:"max_backups" toml:"max_backups"`
	MaxAge     int    `mapstructure:"max_age" toml:"max_age"`
	Compress   bool   `mapstructure:"compress" toml:"compress"`
}

// JWTConfig 定义 JWT 认证配置。
type JWTConfig struct {
	Secret         string        `mapstructure:"secret" toml:"secret"`
	Issuer         string        `mapstructure:"issuer" toml:"issuer"`
	ExpireDuration time.Duration `mapstructure:"expire_duration" toml:"expire_duration"`
}

// SnowflakeConfig 定义雪花算法配置。
type SnowflakeConfig struct {
	StartTime string `mapstructure:"start_time" toml:"start_time"`
	MachineID int64  `mapstructure:"machine_id" toml:"machine_id"`
}

// MessageQueueConfig 定义消息队列配置。
type MessageQueueConfig struct {
	Kafka KafkaConfig `mapstructure:"kafka" toml:"kafka"`
}

// KafkaConfig 定义 Kafka 配置。
type KafkaConfig struct {
	Brokers        []string      `mapstructure:"brokers" toml:"brokers"`
	Topic          string        `mapstructure:"topic" toml:"topic"`
	GroupID        string        `mapstructure:"group_id" toml:"group_id"`
	DialTimeout    time.Duration `mapstructure:"dial_timeout" toml:"dial_timeout"`
	ReadTimeout    time.Duration `mapstructure:"read_timeout" toml:"read_timeout"`
	WriteTimeout   time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`
	MinBytes       int           `mapstructure:"min_bytes" toml:"min_bytes"`
	MaxBytes       int           `mapstructure:"max_bytes" toml:"max_bytes"`
	MaxWait        time.Duration `mapstructure:"max_wait" toml:"max_wait"`
	MaxAttempts    int           `mapstructure:"max_attempts" toml:"max_attempts"`
	CommitInterval time.Duration `mapstructure:"commit_interval" toml:"commit_interval"`
	RequiredAcks   int           `mapstructure:"required_acks" toml:"required_acks"`
	Async          bool          `mapstructure:"async" toml:"async"`
}

// MinioConfig 定义 MinIO 对象存储配置。
type MinioConfig struct {
	Endpoint        string `mapstructure:"endpoint" toml:"endpoint"`
	AccessKeyID     string `mapstructure:"access_key_id" toml:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key" toml:"secret_access_key"`
	UseSSL          bool   `mapstructure:"use_ssl" toml:"use_ssl"`
	BucketName      string `mapstructure:"bucket_name" toml:"bucket_name"`
}

// TracingConfig 定义链路追踪配置。
type TracingConfig struct {
	Enabled      bool   `mapstructure:"enabled" toml:"enabled"`
	ServiceName  string `mapstructure:"service_name" toml:"service_name"`
	OTLPEndpoint string `mapstructure:"otlp_endpoint" toml:"otlp_endpoint"`
}

// MetricsConfig 定义监控指标配置。
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled" toml:"enabled"`
	Port    string `mapstructure:"port" toml:"port"`
	Path    string `mapstructure:"path" toml:"path"`
}

// RateLimitConfig 定义限流配置。
type RateLimitConfig struct {
	Enabled bool `mapstructure:"enabled" toml:"enabled"`
	Rate    int  `mapstructure:"rate" toml:"rate"`
	Burst   int  `mapstructure:"burst" toml:"burst"`
}

// CircuitBreakerConfig 定义熔断器配置。
type CircuitBreakerConfig struct {
	Enabled     bool          `mapstructure:"enabled" toml:"enabled"`
	MaxRequests uint32        `mapstructure:"max_requests" toml:"max_requests"`
	Interval    time.Duration `mapstructure:"interval" toml:"interval"`
	Timeout     time.Duration `mapstructure:"timeout" toml:"timeout"`
}

// CacheConfig 定义缓存配置。
type CacheConfig struct {
	Prefix            string        `mapstructure:"prefix" toml:"prefix"`
	DefaultExpiration time.Duration `mapstructure:"default_expiration" toml:"default_expiration"`
	CleanupInterval   time.Duration `mapstructure:"cleanup_interval" toml:"cleanup_interval"`
}

// LockConfig 定义分布式锁配置。
type LockConfig struct {
	Prefix            string        `mapstructure:"prefix" toml:"prefix"`
	DefaultExpiration time.Duration `mapstructure:"default_expiration" toml:"default_expiration"`
	MaxRetries        int           `mapstructure:"max_retries" toml:"max_retries"`
	RetryDelay        time.Duration `mapstructure:"retry_delay" toml:"retry_delay"`
}

// ServicesConfig 定义服务地址映射。
type ServicesConfig map[string]ServiceAddr

// ServiceAddr 定义单个服务的地址信息。
type ServiceAddr struct {
	GRPCAddr string `mapstructure:"grpc_addr" toml:"grpc_addr"`
	HTTPAddr string `mapstructure:"http_addr" toml:"http_addr"`
}

// HadoopConfig 定义 Hadoop 配置。
type HadoopConfig struct {
	NameNode string `mapstructure:"name_node" toml:"name_node"`
}

// BigCacheConfig 定义 BigCache 配置。
type BigCacheConfig struct {
	Shards             int           `mapstructure:"shards" toml:"shards"`
	LifeWindow         time.Duration `mapstructure:"life_window" toml:"life_window"`
	CleanWindow        time.Duration `mapstructure:"clean_window" toml:"clean_window"`
	MaxEntrySize       int           `mapstructure:"max_entry_size" toml:"max_entry_size"`
	Verbose            bool          `mapstructure:"verbose" toml:"verbose"`
	HardMaxCacheSize   int           `mapstructure:"hard_max_cache_size" toml:"hard_max_cache_size"`
	OnRemoveWithReason bool          `mapstructure:"on_remove_with_reason" toml:"on_remove_with_reason"`
}

// MongoDBConfig 定义 MongoDB 配置。
type MongoDBConfig struct {
	URI      string `mapstructure:"uri" toml:"uri"`
	Database string `mapstructure:"database" toml:"database"`
}

// ClickHouseConfig 定义 ClickHouse 配置。
type ClickHouseConfig struct {
	Addr     string `mapstructure:"addr" toml:"addr"`
	Database string `mapstructure:"database" toml:"database"`
	Username string `mapstructure:"username" toml:"username"`
	Password string `mapstructure:"password" toml:"password"`
}

// Neo4jConfig 定义 Neo4j 配置。
type Neo4jConfig struct {
	URI      string `mapstructure:"uri" toml:"uri"`
	Username string `mapstructure:"username" toml:"username"`
	Password string `mapstructure:"password" toml:"password"`
}

// ElasticsearchConfig 定义 Elasticsearch 配置。
type ElasticsearchConfig struct {
	Addresses []string `mapstructure:"addresses" toml:"addresses"`
	Username  string   `mapstructure:"username" toml:"username"`
	Password  string   `mapstructure:"password" toml:"password"`
}

// Load 从指定路径加载配置。
func Load(path string, conf interface{}) error {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")
	v.AutomaticEnv()
	v.SetEnvPrefix("ECOMMERCE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	if err := v.Unmarshal(conf); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}

	// 校验配置
	validate := validator.New()
	if err := validate.Struct(conf); err != nil {
		return fmt.Errorf("配置校验失败: %w", err)
	}

	// 监听文件变更（热加载）
	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		slog.Info("配置文件已变更", "file", e.Name)
		if err := v.Unmarshal(conf); err != nil {
			slog.Error("变更后解析配置失败", "error", err)
			return
		}
		// 变更后重新校验
		if err := validate.Struct(conf); err != nil {
			slog.Error("变更后配置校验失败", "error", err)
		}
	})

	return nil
}
