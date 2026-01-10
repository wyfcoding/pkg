// Package config 提供了统一 the 配置加载与管理能力.
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

// Config 全局顶级配置结构.
// 字段已按内存对齐优化 (fieldalignment).
type Config struct {
	Services       ServicesConfig       `mapstructure:"services"       toml:"services"`
	Hadoop         HadoopConfig         `mapstructure:"hadoop"         toml:"hadoop"`
	Version        string               `mapstructure:"version"        toml:"version"`
	Minio          MinioConfig          `mapstructure:"minio"          toml:"minio"`
	Tracing        TracingConfig        `mapstructure:"tracing"        toml:"tracing"`
	Metrics        MetricsConfig        `mapstructure:"metrics"        toml:"metrics"`
	JWT            JWTConfig            `mapstructure:"jwt"            toml:"jwt"`
	Snowflake      SnowflakeConfig      `mapstructure:"snowflake"      toml:"snowflake"`
	Cache          CacheConfig          `mapstructure:"cache"          toml:"cache"`
	Lock           LockConfig           `mapstructure:"lock"           toml:"lock"`
	Log            LogConfig            `mapstructure:"log"            toml:"log"`
	Server         ServerConfig         `mapstructure:"server"         toml:"server"`
	MessageQueue   MessageQueueConfig   `mapstructure:"messagequeue"   toml:"messagequeue"`
	Data           DataConfig           `mapstructure:"data"           toml:"data"`
	RateLimit      RateLimitConfig      `mapstructure:"ratelimit"      toml:"ratelimit"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuitbreaker" toml:"circuitbreaker"`
}

// ServerConfig 定义服务器运行时的基础网络与环境参数.
type ServerConfig struct {
	Name        string `mapstructure:"name"        toml:"name"        validate:"required"`
	Environment string `mapstructure:"environment" toml:"environment" validate:"oneof=dev test prod"`
	HTTP        struct {
		Addr         string        `mapstructure:"addr"          toml:"addr"`
		Timeout      time.Duration `mapstructure:"timeout"       toml:"timeout"`
		ReadTimeout  time.Duration `mapstructure:"read_timeout"  toml:"read_timeout"`
		WriteTimeout time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`
		IdleTimeout  time.Duration `mapstructure:"idle_timeout"  toml:"idle_timeout"`
		Port         int           `mapstructure:"port"          toml:"port"          validate:"required,min=1,max=65535"`
	} `mapstructure:"http" toml:"http"`
	GRPC struct {
		Addr                 string        `mapstructure:"addr"                   toml:"addr"`
		Timeout              time.Duration `mapstructure:"timeout"                toml:"timeout"`
		Port                 int           `mapstructure:"port"                   toml:"port"                   validate:"required,min=1,max=65535"`
		MaxRecvMsgSize       int           `mapstructure:"max_recv_msg_size"      toml:"max_recv_msg_size"`
		MaxSendMsgSize       int           `mapstructure:"max_send_msg_size"      toml:"max_send_msg_size"`
		MaxConcurrentStreams int           `mapstructure:"max_concurrent_streams" toml:"max_concurrent_streams"`
	} `mapstructure:"grpc" toml:"grpc"`
}

// DataConfig 汇集了所有持久化存储与中间件的数据源配置.
type DataConfig struct {
	ClickHouse    ClickHouseConfig    `mapstructure:"clickhouse"    toml:"clickhouse"`
	Elasticsearch ElasticsearchConfig `mapstructure:"elasticsearch" toml:"elasticsearch"`
	Neo4j         Neo4jConfig         `mapstructure:"neo4j"         toml:"neo4j"`
	MongoDB       MongoDBConfig       `mapstructure:"mongodb"       toml:"mongodb"`
	Shards        []DatabaseConfig    `mapstructure:"shards"        toml:"shards"`
	Database      DatabaseConfig      `mapstructure:"database"      toml:"database"`
	Redis         RedisConfig         `mapstructure:"redis"         toml:"redis"`
	BigCache      BigCacheConfig      `mapstructure:"bigcache"      toml:"bigcache"`
}

// DatabaseConfig 定义单数据库实例连接与连接池参数.
type DatabaseConfig struct {
	Driver          string          `mapstructure:"driver"                 toml:"driver"            validate:"required"`
	DSN             string          `mapstructure:"dsn"               toml:"dsn"               validate:"required"`
	ConnMaxLifetime time.Duration   `mapstructure:"conn_max_lifetime"  toml:"conn_max_lifetime"`
	SlowThreshold   time.Duration   `mapstructure:"slow_threshold"     toml:"slow_threshold"`
	LogLevel        logger.LogLevel `mapstructure:"log_level"          toml:"log_level"`
	MaxIdleConns    int             `mapstructure:"max_idle_conns"     toml:"max_idle_conns"`
	MaxOpenConns    int             `mapstructure:"max_open_conns"     toml:"max_open_conns"`
}

// RedisConfig 定义 Redis 连接与池化参数.
type RedisConfig struct {
	Addr         string        `mapstructure:"addr"          toml:"addr" validate:"required"`
	Password     string        `mapstructure:"password"      toml:"password"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"  toml:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`
	DB           int           `mapstructure:"db"            toml:"db"`
	PoolSize     int           `mapstructure:"pool_size"     toml:"pool_size"`
	MinIdleConns int           `mapstructure:"min_idle_conns" toml:"min_idle_conns"`
}

// LogConfig 定义日志输出、级别与切割策略.
type LogConfig struct {
	Level      string `mapstructure:"level"       toml:"level"`
	Format     string `mapstructure:"format"      toml:"format"`
	Output     string `mapstructure:"output"      toml:"output"`
	File       string `mapstructure:"file"        toml:"file"`
	MaxSize    int    `mapstructure:"max_size"    toml:"max_size"`
	MaxBackups int    `mapstructure:"max_backups" toml:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"     toml:"max_age"`
	Compress   bool   `mapstructure:"compress"    toml:"compress"`
}

// JWTConfig 身份认证令牌相关配置.
type JWTConfig struct {
	Secret         string        `mapstructure:"secret"          toml:"secret"`
	Issuer         string        `mapstructure:"issuer"          toml:"issuer"`
	ExpireDuration time.Duration `mapstructure:"expire_duration" toml:"expire_duration"`
}

// SnowflakeConfig 雪花算法分布式 ID 生成器参数.
type SnowflakeConfig struct {
	StartTime string `mapstructure:"start_time" toml:"start_time"`
	Type      string `mapstructure:"type"       toml:"type"`
	MachineID int64  `mapstructure:"machine_id" toml:"machine_id"`
}

// MessageQueueConfig 聚合所有消息中间件配置.
type MessageQueueConfig struct {
	Kafka KafkaConfig `mapstructure:"kafka" toml:"kafka"`
}

// KafkaConfig 定义 Kafka 生产者与消费者参数.
type KafkaConfig struct {
	Topic          string        `mapstructure:"topic"           toml:"topic"`
	GroupID        string        `mapstructure:"group_id"        toml:"group_id"`
	Brokers        []string      `mapstructure:"brokers"         toml:"brokers"`
	DialTimeout    time.Duration `mapstructure:"dial_timeout"    toml:"dial_timeout"`
	ReadTimeout    time.Duration `mapstructure:"read_timeout"    toml:"read_timeout"`
	WriteTimeout   time.Duration `mapstructure:"write_timeout"   toml:"write_timeout"`
	MaxWait        time.Duration `mapstructure:"max_wait"        toml:"max_wait"`
	CommitInterval time.Duration `mapstructure:"commit_interval" toml:"commit_interval"`
	MinBytes       int           `mapstructure:"min_bytes"       toml:"min_bytes"`
	MaxBytes       int           `mapstructure:"max_bytes"       toml:"max_bytes"`
	MaxAttempts    int           `mapstructure:"max_attempts"    toml:"max_attempts"`
	RequiredAcks   int           `mapstructure:"required_acks"   toml:"required_acks"`
	Async          bool          `mapstructure:"async"           toml:"async"`
}

// MinioConfig 定义 S3 兼容对象存储 MinIO 的连接参数.
type MinioConfig struct {
	Endpoint        string `mapstructure:"endpoint"          toml:"endpoint"`
	AccessKeyID     string `mapstructure:"access_key_id"     toml:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key" toml:"secret_access_key"`
	BucketName      string `mapstructure:"bucket_name"       toml:"bucket_name"`
	UseSSL          bool   `mapstructure:"use_ssl"           toml:"use_ssl"`
}

// TracingConfig 分布式链路追踪（OpenTelemetry/Jaeger）配置.
type TracingConfig struct {
	ServiceName  string `mapstructure:"service_name"  toml:"service_name"`
	OTLPEndpoint string `mapstructure:"otlp_endpoint" toml:"otlp_endpoint"`
	Enabled      bool   `mapstructure:"enabled"       toml:"enabled"`
}

// MetricsConfig 普罗米修斯监控指标暴露配置.
type MetricsConfig struct {
	Port    string `mapstructure:"port"    toml:"port"`
	Path    string `mapstructure:"path"    toml:"path"`
	Enabled bool   `mapstructure:"enabled" toml:"enabled"`
}

// RateLimitConfig 定义令牌桶或漏桶限流参数.
type RateLimitConfig struct {
	Rate    int  `mapstructure:"rate"    toml:"rate"`
	Burst   int  `mapstructure:"burst"   toml:"burst"`
	Enabled bool `mapstructure:"enabled" toml:"enabled"`
}

// CircuitBreakerConfig 定义熔断器（如 Gobreaker）的保护策略.
type CircuitBreakerConfig struct {
	Interval    time.Duration `mapstructure:"interval"     toml:"interval"`
	Timeout     time.Duration `mapstructure:"timeout"      toml:"timeout"`
	MaxRequests uint32        `mapstructure:"max_requests" toml:"max_requests"`
	Enabled     bool          `mapstructure:"enabled"      toml:"enabled"`
}

// CacheConfig 通用缓存策略配置.
type CacheConfig struct {
	Prefix            string        `mapstructure:"prefix"             toml:"prefix"`
	DefaultExpiration time.Duration `mapstructure:"default_expiration" toml:"default_expiration"`
	CleanupInterval   time.Duration `mapstructure:"cleanup_interval"   toml:"cleanup_interval"`
}

// LockConfig 分布式锁通用配置.
type LockConfig struct {
	Prefix            string        `mapstructure:"prefix"             toml:"prefix"`
	DefaultExpiration time.Duration `mapstructure:"default_expiration" toml:"default_expiration"`
	RetryDelay        time.Duration `mapstructure:"retry_delay"        toml:"retry_delay"`
	MaxRetries        int           `mapstructure:"max_retries"        toml:"max_retries"`
}

// ServicesConfig 下游依赖微服务的地址注册映射.
type ServicesConfig map[string]ServiceAddr

// ServiceAddr 定义单个微服务的接入地址.
type ServiceAddr struct {
	GRPCAddr string `mapstructure:"grpc_addr" toml:"grpc_addr"`
	HTTPAddr string `mapstructure:"http_addr" toml:"http_addr"`
}

// HadoopConfig 大数据生态组件 HDFS 配置.
type HadoopConfig struct {
	NameNode string `mapstructure:"name_node" toml:"name_node"`
}

// BigCacheConfig 高性能本地内存缓存参数.
type BigCacheConfig struct {
	LifeWindow         time.Duration `mapstructure:"life_window"           toml:"life_window"`
	CleanWindow        time.Duration `mapstructure:"clean_window"          toml:"clean_window"`
	Shards             int           `mapstructure:"shards"                toml:"shards"`
	MaxEntrySize       int           `mapstructure:"max_entry_size"        toml:"max_entry_size"`
	HardMaxCacheSize   int           `mapstructure:"hard_max_cache_size"   toml:"hard_max_cache_size"`
	Verbose            bool          `mapstructure:"verbose"               toml:"verbose"`
	OnRemoveWithReason bool          `mapstructure:"on_remove_with_reason" toml:"on_remove_with_reason"`
}

// MongoDBConfig 定义 MongoDB 的连接参数.
type MongoDBConfig struct {
	URI      string `mapstructure:"uri"      toml:"uri"`
	Database string `mapstructure:"database" toml:"database"`
}

// ClickHouseConfig 定义 ClickHouse 列式存储的连接参数.
type ClickHouseConfig struct {
	Addr     string `mapstructure:"addr"     toml:"addr"`
	Database string `mapstructure:"database" toml:"database"`
	Username string `mapstructure:"username" toml:"username"`
	Password string `mapstructure:"password" toml:"password"`
}

// Neo4jConfig 定义 Neo4j 图数据库的连接参数.
type Neo4jConfig struct {
	URI      string `mapstructure:"uri"      toml:"uri"`
	Username string `mapstructure:"username" toml:"username"`
	Password string `mapstructure:"password" toml:"password"`
}

// ElasticsearchConfig 定义搜索引擎集群连接参数.
type ElasticsearchConfig struct {
	Username  string   `mapstructure:"username"  toml:"username"`
	Password  string   `mapstructure:"password"  toml:"password"`
	Addresses []string `mapstructure:"addresses" toml:"addresses"`
}

var vInstance = viper.New()

// Load 全生产级的配置加载逻辑.
func Load(path string, conf any) error {
	vInstance.SetConfigFile(path)
	vInstance.SetConfigType("toml")

	vInstance.SetEnvPrefix("APP")
	vInstance.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	vInstance.AutomaticEnv()

	if err := vInstance.ReadInConfig(); err != nil {
		return fmt.Errorf("read config error: %w", err)
	}

	if err := vInstance.Unmarshal(conf); err != nil {
		return fmt.Errorf("unmarshal config error: %w", err)
	}

	validate := validator.New()
	if err := validate.Struct(conf); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	vInstance.WatchConfig()
	vInstance.OnConfigChange(func(event fsnotify.Event) {
		slog.Info("detecting config change", "file", event.Name)
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

// PrintWithMask 脱敏打印当前配置.
func PrintWithMask(conf any) {
	data, err := json.Marshal(conf)
	if err != nil {
		slog.Error("failed to marshal config for printing", "error", err)

		return
	}

	var configMap map[string]any
	if unmarshalErr := json.Unmarshal(data, &configMap); unmarshalErr != nil {
		slog.Error("failed to unmarshal config for masking", "error", unmarshalErr)

		return
	}

	mask(configMap)

	maskedJSON, marshalErr := json.MarshalIndent(configMap, "  ", "  ")
	if marshalErr != nil {
		slog.Error("failed to marshal masked config", "error", marshalErr)

		return
	}

	slog.Info("Current effective configuration", "config", string(maskedJSON))
}

func mask(configMap map[string]any) {
	sensitiveKeys := []string{"password", "secret", "dsn", "key", "token"}

	for key, val := range configMap {
		if subMap, ok := val.(map[string]any); ok {
			mask(subMap)

			continue
		}

		if slice, ok := val.([]any); ok {
			for _, item := range slice {
				if itemMap, ok := item.(map[string]any); ok {
					mask(itemMap)
				}
			}

			continue
		}

		for _, sensitiveKey := range sensitiveKeys {
			if strings.Contains(strings.ToLower(key), sensitiveKey) {
				configMap[key] = "******"

				break
			}
		}
	}
}

// GetViper 返回底层的 Viper 实例.
func GetViper() *viper.Viper {
	return vInstance
}
