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

// Config 全局顶级配置结构
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

// ... (ServerConfig, DataConfig 等子结构体定义保持不变，以确保向后兼容) ...

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

type DatabaseConfig struct {
	Driver          string          `mapstructure:"driver" toml:"driver" validate:"required"`
	DSN             string          `mapstructure:"dsn" toml:"dsn" validate:"required"`
	MaxIdleConns    int             `mapstructure:"max_idle_conns" toml:"max_idle_conns"`
	MaxOpenConns    int             `mapstructure:"max_open_conns" toml:"max_open_conns"`
	ConnMaxLifetime time.Duration   `mapstructure:"conn_max_lifetime" toml:"conn_max_lifetime"`
	LogLevel        logger.LogLevel `mapstructure:"log_level" toml:"log_level"`
	SlowThreshold   time.Duration   `mapstructure:"slow_threshold" toml:"slow_threshold"`
}

type RedisConfig struct {
	Addr         string        `mapstructure:"addr" toml:"addr" validate:"required"`
	Password     string        `mapstructure:"password" toml:"password"`
	DB           int           `mapstructure:"db" toml:"db"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" toml:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`
	PoolSize     int           `mapstructure:"pool_size" toml:"pool_size"`
	MinIdleConns int           `mapstructure:"min_idle_conns" toml:"min_idle_conns"`
}

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

type JWTConfig struct {
	Secret         string        `mapstructure:"secret" toml:"secret"`
	Issuer         string        `mapstructure:"issuer" toml:"issuer"`
	ExpireDuration time.Duration `mapstructure:"expire_duration" toml:"expire_duration"`
}

type SnowflakeConfig struct {
	Type      string `mapstructure:"type" toml:"type"`
	StartTime string `mapstructure:"start_time" toml:"start_time"`
	MachineID int64  `mapstructure:"machine_id" toml:"machine_id"`
}

type MessageQueueConfig struct {
	Kafka KafkaConfig `mapstructure:"kafka" toml:"kafka"`
}

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

type MinioConfig struct {
	Endpoint        string `mapstructure:"endpoint" toml:"endpoint"`
	AccessKeyID     string `mapstructure:"access_key_id" toml:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key" toml:"secret_access_key"`
	UseSSL          bool   `mapstructure:"use_ssl" toml:"use_ssl"`
	BucketName      string `mapstructure:"bucket_name" toml:"bucket_name"`
}

type TracingConfig struct {
	Enabled      bool   `mapstructure:"enabled" toml:"enabled"`
	ServiceName  string `mapstructure:"service_name" toml:"service_name"`
	OTLPEndpoint string `mapstructure:"otlp_endpoint" toml:"otlp_endpoint"`
}

type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled" toml:"enabled"`
	Port    string `mapstructure:"port" toml:"port"`
	Path    string `mapstructure:"path" toml:"path"`
}

type RateLimitConfig struct {
	Enabled bool `mapstructure:"enabled" toml:"enabled"`
	Rate    int  `mapstructure:"rate" toml:"rate"`
	Burst   int  `mapstructure:"burst" toml:"burst"`
}

type CircuitBreakerConfig struct {
	Enabled     bool          `mapstructure:"enabled" toml:"enabled"`
	MaxRequests uint32        `mapstructure:"max_requests" toml:"max_requests"`
	Interval    time.Duration `mapstructure:"interval" toml:"interval"`
	Timeout     time.Duration `mapstructure:"timeout" toml:"timeout"`
}

type CacheConfig struct {
	Prefix            string        `mapstructure:"prefix" toml:"prefix"`
	DefaultExpiration time.Duration `mapstructure:"default_expiration" toml:"default_expiration"`
	CleanupInterval   time.Duration `mapstructure:"cleanup_interval" toml:"cleanup_interval"`
}

type LockConfig struct {
	Prefix            string        `mapstructure:"prefix" toml:"prefix"`
	DefaultExpiration time.Duration `mapstructure:"default_expiration" toml:"default_expiration"`
	MaxRetries        int           `mapstructure:"max_retries" toml:"max_retries"`
	RetryDelay        time.Duration `mapstructure:"retry_delay" toml:"retry_delay"`
}

type ServicesConfig map[string]ServiceAddr

type ServiceAddr struct {
	GRPCAddr string `mapstructure:"grpc_addr" toml:"grpc_addr"`
	HTTPAddr string `mapstructure:"http_addr" toml:"http_addr"`
}

type HadoopConfig struct {
	NameNode string `mapstructure:"name_node" toml:"name_node"`
}

type BigCacheConfig struct {
	Shards             int           `mapstructure:"shards" toml:"shards"`
	LifeWindow         time.Duration `mapstructure:"life_window" toml:"life_window"`
	CleanWindow        time.Duration `mapstructure:"clean_window" toml:"clean_window"`
	MaxEntrySize       int           `mapstructure:"max_entry_size" toml:"max_entry_size"`
	Verbose            bool          `mapstructure:"verbose" toml:"verbose"`
	HardMaxCacheSize   int           `mapstructure:"hard_max_cache_size" toml:"hard_max_cache_size"`
	OnRemoveWithReason bool          `mapstructure:"on_remove_with_reason" toml:"on_remove_with_reason"`
}

type MongoDBConfig struct {
	URI      string `mapstructure:"uri" toml:"uri"`
	Database string `mapstructure:"database" toml:"database"`
}

type ClickHouseConfig struct {
	Addr     string `mapstructure:"addr" toml:"addr"`
	Database string `mapstructure:"database" toml:"database"`
	Username string `mapstructure:"username" toml:"username"`
	Password string `mapstructure:"password" toml:"password"`
}

type Neo4jConfig struct {
	URI      string `mapstructure:"uri" toml:"uri"`
	Username string `mapstructure:"username" toml:"username"`
	Password string `mapstructure:"password" toml:"password"`
}

type ElasticsearchConfig struct {
	Addresses []string `mapstructure:"addresses" toml:"addresses"`
	Username  string   `mapstructure:"username" toml:"username"`
	Password  string   `mapstructure:"password" toml:"password"`
}

var v = viper.New()

// Load 全生产级的配置加载逻辑
func Load(path string, conf any) error {
	v.SetConfigFile(path)
	v.SetConfigType("toml")

	// 1. 设置环境变量支持
	v.SetEnvPrefix("APP") // 环境变量前缀 APP_
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 2. 加载基础文件
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config error: %w", err)
	}

	// 3. 初始解析
	if err := v.Unmarshal(conf); err != nil {
		return fmt.Errorf("unmarshal config error: %w", err)
	}

	// 4. 实时校验
	validate := validator.New()
	if err := validate.Struct(conf); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// 5. 开启热加载 (带防抖逻辑)
	v.WatchConfig()
	v.OnConfigChange(func(e fsnotify.Event) {
		slog.Info("detecting config change", "file", e.Name)
		// 延迟 500ms 确保文件写操作已完成 (针对某些编辑器的原子保存行为)
		time.Sleep(500 * time.Millisecond)
		if err := v.Unmarshal(conf); err != nil {
			slog.Error("reload config unmarshal failed", "error", err)
			return
		}
		if err := validate.Struct(conf); err != nil {
			slog.Error("reload config validation failed", "error", err)
		} else {
			slog.Info("config hot-reloaded and validated successfully")
		}
	})

	return nil
}

// PrintWithMask 脱敏打印当前配置，方便生产环境排查
func PrintWithMask(conf any) {
	data, err := json.Marshal(conf)
	if err != nil {
		slog.Error("failed to marshal config for printing", "error", err)
		return
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		slog.Error("failed to unmarshal config for masking", "error", err)
		return
	}

	// 简单的脱敏递归函数
	mask(m)

	maskedJSON, err := json.MarshalIndent(m, "  ", "  ")
	if err != nil {
		slog.Error("failed to marshal masked config", "error", err)
		return
	}
	slog.Info("Current effective configuration", "config", string(maskedJSON))
}

func mask(m map[string]any) {
	sensitiveKeys := []string{"password", "secret", "dsn", "key", "token"}
	for k, v := range m {
		// 递归处理子 Map
		if subMap, ok := v.(map[string]any); ok {
			mask(subMap)
			continue
		}
		// 递归处理数组中的 Map
		if slice, ok := v.([]any); ok {
			for _, item := range slice {
				if itemMap, ok := item.(map[string]any); ok {
					mask(itemMap)
				}
			}
			continue
		}

		// 执行脱敏
		for _, sk := range sensitiveKeys {
			if strings.Contains(strings.ToLower(k), sk) {
				m[k] = "******"
				break
			}
		}
	}
}

// GetViper 返回底层的 Viper 实例，供需要直接读取动态值的场景使用
func GetViper() *viper.Viper {
	return v
}
