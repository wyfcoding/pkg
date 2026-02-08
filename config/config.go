// Package config 提供了统一 the 配置加载与管理能力.
// 生成摘要:
// 1) 增加远程日志配置结构体并挂载到 LogConfig。
// 2) 增加 HTTP/gRPC 慢请求阈值配置。
// 3) 增加 HTTP/gRPC 并发限制配置结构。
// 4) 增加 HTTP 请求体大小限制配置。
// 假设:
// 1) 远程日志为可选配置，默认关闭。
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/wyfcoding/pkg/logging"

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
	Concurrency    ConcurrencyConfig    `mapstructure:"concurrency"    toml:"concurrency"`
	CORS           CORSConfig           `mapstructure:"cors"           toml:"cors"`
	Security       SecurityConfig       `mapstructure:"security"       toml:"security"`
	HTTPClient     HTTPClientConfig     `mapstructure:"httpclient"     toml:"httpclient"`
	Maintenance    MaintenanceConfig    `mapstructure:"maintenance"    toml:"maintenance"`
	FeatureFlags   FeatureFlagConfig    `mapstructure:"feature_flags"  toml:"feature_flags"`
}

// ServerConfig 定义服务器运行时的基础网络与环境参数.
type ServerConfig struct {
	Name        string `mapstructure:"name"        toml:"name"        validate:"required"`
	Environment string `mapstructure:"environment" toml:"environment" validate:"oneof=dev test prod"`
	HTTP        struct {
		Addr              string        `mapstructure:"addr"          toml:"addr"`
		Timeout           time.Duration `mapstructure:"timeout"       toml:"timeout"`
		ReadTimeout       time.Duration `mapstructure:"read_timeout"  toml:"read_timeout"`
		ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout" toml:"read_header_timeout"`
		WriteTimeout      time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`
		IdleTimeout       time.Duration `mapstructure:"idle_timeout"  toml:"idle_timeout"`
		MaxHeaderBytes    int           `mapstructure:"max_header_bytes" toml:"max_header_bytes"`
		MaxBodyBytes      int64         `mapstructure:"max_body_bytes" toml:"max_body_bytes"`
		TrustedProxies    []string      `mapstructure:"trusted_proxies" toml:"trusted_proxies"`
		Port              int           `mapstructure:"port"          toml:"port"          validate:"required,min=1,max=65535"`
	} `mapstructure:"http" toml:"http"`
	GRPC struct {
		Addr                 string              `mapstructure:"addr"                   toml:"addr"`
		Timeout              time.Duration       `mapstructure:"timeout"                toml:"timeout"`
		Port                 int                 `mapstructure:"port"                   toml:"port"                   validate:"required,min=1,max=65535"`
		MaxRecvMsgSize       int                 `mapstructure:"max_recv_msg_size"      toml:"max_recv_msg_size"`
		MaxSendMsgSize       int                 `mapstructure:"max_send_msg_size"      toml:"max_send_msg_size"`
		MaxConcurrentStreams int                 `mapstructure:"max_concurrent_streams" toml:"max_concurrent_streams"`
		Keepalive            GRPCKeepaliveConfig `mapstructure:"keepalive"          toml:"keepalive"`
	} `mapstructure:"grpc" toml:"grpc"`
}

// DataConfig 汇集了所有持久化存储与中间件的数据源配置.
type DataConfig struct {
	ClickHouse    ClickHouseConfig    `mapstructure:"clickhouse"    toml:"clickhouse"`
	Elasticsearch ElasticsearchConfig `mapstructure:"elasticsearch" toml:"elasticsearch"`
	Neo4j         Neo4jConfig         `mapstructure:"neo4j"         toml:"neo4j"`
	MongoDB       MongoDBConfig       `mapstructure:"mongodb"       toml:"mongodb"`
	Etcd          EtcdConfig          `mapstructure:"etcd"          toml:"etcd"`
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
	MasterName   string        `mapstructure:"master_name"   toml:"master_name"`
	Password     string        `mapstructure:"password"      toml:"password"`
	Addrs        []string      `mapstructure:"addrs"         toml:"addrs"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"  toml:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" toml:"write_timeout"`
	DB           int           `mapstructure:"db"            toml:"db"`
	PoolSize     int           `mapstructure:"pool_size"     toml:"pool_size"`
	MinIdleConns int           `mapstructure:"min_idle_conns" toml:"min_idle_conns"`
}

// LogConfig 定义日志输出、级别与切割策略.
type LogConfig struct {
	Level             string          `mapstructure:"level"              toml:"level"`                // 日志级别。
	Format            string          `mapstructure:"format"             toml:"format"`               // 日志格式（json/text）。
	Output            string          `mapstructure:"output"             toml:"output"`               // 日志输出目标。
	File              string          `mapstructure:"file"               toml:"file"`                 // 日志文件路径。
	MaxSize           int             `mapstructure:"max_size"           toml:"max_size"`             // 单个文件最大大小 (MB)。
	MaxBackups        int             `mapstructure:"max_backups"        toml:"max_backups"`          // 最大备份数。
	MaxAge            int             `mapstructure:"max_age"            toml:"max_age"`              // 最大保留天数。
	Compress          bool            `mapstructure:"compress"           toml:"compress"`             // 是否启用压缩。
	SlowThreshold     time.Duration   `mapstructure:"slow_threshold"      toml:"slow_threshold"`      // HTTP 慢请求阈值。
	GRPCSlowThreshold time.Duration   `mapstructure:"grpc_slow_threshold" toml:"grpc_slow_threshold"` // gRPC 慢请求阈值。
	Remote            RemoteLogConfig `mapstructure:"remote"             toml:"remote"`               // 远程日志写入配置。
}

// RemoteLogConfig 定义远程日志写入配置。
type RemoteLogConfig struct {
	Enabled       bool          `mapstructure:"enabled"        toml:"enabled"`
	Endpoint      string        `mapstructure:"endpoint"       toml:"endpoint"`
	AuthToken     string        `mapstructure:"auth_token"     toml:"auth_token"`
	Timeout       time.Duration `mapstructure:"timeout"        toml:"timeout"`
	BatchSize     int           `mapstructure:"batch_size"     toml:"batch_size"`
	BufferSize    int           `mapstructure:"buffer_size"    toml:"buffer_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval" toml:"flush_interval"`
	DropOnFull    bool          `mapstructure:"drop_on_full"   toml:"drop_on_full"`
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
	Topic           string        `mapstructure:"topic"           toml:"topic"`
	GroupID         string        `mapstructure:"group_id"        toml:"group_id"`
	Brokers         []string      `mapstructure:"brokers"         toml:"brokers"`
	DialTimeout     time.Duration `mapstructure:"dial_timeout"    toml:"dial_timeout"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"    toml:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"   toml:"write_timeout"`
	MaxWait         time.Duration `mapstructure:"max_wait"        toml:"max_wait"`
	CommitInterval  time.Duration `mapstructure:"commit_interval" toml:"commit_interval"`
	MinBytes        int           `mapstructure:"min_bytes"       toml:"min_bytes"`
	MaxBytes        int           `mapstructure:"max_bytes"       toml:"max_bytes"`
	MaxAttempts     int           `mapstructure:"max_attempts"    toml:"max_attempts"`
	RequiredAcks    int           `mapstructure:"required_acks"   toml:"required_acks"`
	Async           bool          `mapstructure:"async"           toml:"async"`
	RetryMax        int           `mapstructure:"retry_max"        toml:"retry_max"`
	RetryInitial    time.Duration `mapstructure:"retry_initial"    toml:"retry_initial"`
	RetryMaxBackoff time.Duration `mapstructure:"retry_max_backoff" toml:"retry_max_backoff"`
	RetryMultiplier float64       `mapstructure:"retry_multiplier" toml:"retry_multiplier"`
	RetryJitter     float64       `mapstructure:"retry_jitter"     toml:"retry_jitter"`
	DLQEnabled      bool          `mapstructure:"dlq_enabled"      toml:"dlq_enabled"`
	DLQTopic        string        `mapstructure:"dlq_topic"        toml:"dlq_topic"`
	CommitOnError   bool          `mapstructure:"commit_on_error"  toml:"commit_on_error"`
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
	ServiceName  string  `mapstructure:"service_name"  toml:"service_name"`
	OTLPEndpoint string  `mapstructure:"otlp_endpoint" toml:"otlp_endpoint"`
	SamplerRatio float64 `mapstructure:"sampler_ratio" toml:"sampler_ratio"`
	Enabled      bool    `mapstructure:"enabled"       toml:"enabled"`
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

// CORSConfig 定义跨域配置。
type CORSConfig struct {
	Enabled          bool          `mapstructure:"enabled"           toml:"enabled"`
	AllowOrigins     []string      `mapstructure:"allow_origins"      toml:"allow_origins"`
	AllowMethods     []string      `mapstructure:"allow_methods"      toml:"allow_methods"`
	AllowHeaders     []string      `mapstructure:"allow_headers"      toml:"allow_headers"`
	ExposeHeaders    []string      `mapstructure:"expose_headers"     toml:"expose_headers"`
	AllowCredentials bool          `mapstructure:"allow_credentials"  toml:"allow_credentials"`
	MaxAge           time.Duration `mapstructure:"max_age"            toml:"max_age"`
}

// SecurityConfig 定义 HTTP 安全响应头配置。
type SecurityConfig struct {
	Enabled                   bool     `mapstructure:"enabled"                        toml:"enabled"`
	FrameOptions              string   `mapstructure:"frame_options"                   toml:"frame_options"`
	ContentTypeOptions        string   `mapstructure:"content_type_options"            toml:"content_type_options"`
	XSSProtection             string   `mapstructure:"xss_protection"                  toml:"xss_protection"`
	ReferrerPolicy            string   `mapstructure:"referrer_policy"                 toml:"referrer_policy"`
	ContentSecurityPolicy     string   `mapstructure:"content_security_policy"         toml:"content_security_policy"`
	PermissionsPolicy         string   `mapstructure:"permissions_policy"              toml:"permissions_policy"`
	HSTSMaxAge                int      `mapstructure:"hsts_max_age"                    toml:"hsts_max_age"`
	HSTSIncludeSubdomains     bool     `mapstructure:"hsts_include_subdomains"         toml:"hsts_include_subdomains"`
	HSTSPreload               bool     `mapstructure:"hsts_preload"                    toml:"hsts_preload"`
	AdditionalHeaders         []string `mapstructure:"additional_headers"              toml:"additional_headers"`
	AdditionalHeaderSeparator string   `mapstructure:"additional_header_separator"     toml:"additional_header_separator"`
	IPAllowlist               []string `mapstructure:"ip_allowlist"                    toml:"ip_allowlist"`
}

// HTTPClientConfig 定义统一 HTTP 客户端配置。
type HTTPClientConfig struct {
	Timeout         time.Duration        `mapstructure:"timeout"             toml:"timeout"`
	RateLimit       int                  `mapstructure:"rate_limit"          toml:"rate_limit"`
	RateBurst       int                  `mapstructure:"rate_burst"          toml:"rate_burst"`
	MaxConcurrency  int                  `mapstructure:"max_concurrency"     toml:"max_concurrency"`
	SlowThreshold   time.Duration        `mapstructure:"slow_threshold"      toml:"slow_threshold"`
	RetryMax        int                  `mapstructure:"retry_max"           toml:"retry_max"`
	RetryInitial    time.Duration        `mapstructure:"retry_initial"       toml:"retry_initial"`
	RetryMaxBackoff time.Duration        `mapstructure:"retry_max_backoff"   toml:"retry_max_backoff"`
	RetryMultiplier float64              `mapstructure:"retry_multiplier"    toml:"retry_multiplier"`
	RetryJitter     float64              `mapstructure:"retry_jitter"        toml:"retry_jitter"`
	RetryStatus     []int                `mapstructure:"retry_status"        toml:"retry_status"`
	RetryMethods    []string             `mapstructure:"retry_methods"       toml:"retry_methods"`
	Breaker         CircuitBreakerConfig `mapstructure:"breaker"             toml:"breaker"`
}

// MaintenanceConfig 定义服务维护模式配置。
type MaintenanceConfig struct {
	Enabled    bool     `mapstructure:"enabled"     toml:"enabled"`
	Message    string   `mapstructure:"message"     toml:"message"`
	AllowPaths []string `mapstructure:"allow_paths" toml:"allow_paths"`
}

// FeatureFlagConfig 定义功能开关配置。
type FeatureFlagConfig struct {
	Enabled        bool          `mapstructure:"enabled"         toml:"enabled"`
	DefaultEnabled bool          `mapstructure:"default_enabled" toml:"default_enabled"`
	Flags          []FeatureFlag `mapstructure:"flags"           toml:"flags"`
}

// FeatureFlag 定义单个功能开关。
type FeatureFlag struct {
	Name           string    `mapstructure:"name"             toml:"name"`
	Enabled        bool      `mapstructure:"enabled"          toml:"enabled"`
	Rollout        int       `mapstructure:"rollout"          toml:"rollout"`
	StartAt        time.Time `mapstructure:"start_at"         toml:"start_at"`
	EndAt          time.Time `mapstructure:"end_at"           toml:"end_at"`
	AllowUsers     []string  `mapstructure:"allow_users"      toml:"allow_users"`
	DenyUsers      []string  `mapstructure:"deny_users"       toml:"deny_users"`
	AllowTenants   []string  `mapstructure:"allow_tenants"    toml:"allow_tenants"`
	DenyTenants    []string  `mapstructure:"deny_tenants"     toml:"deny_tenants"`
	AllowRoles     []string  `mapstructure:"allow_roles"      toml:"allow_roles"`
	DenyRoles      []string  `mapstructure:"deny_roles"       toml:"deny_roles"`
	AllowIPs       []string  `mapstructure:"allow_ips"        toml:"allow_ips"`
	DenyIPs        []string  `mapstructure:"deny_ips"         toml:"deny_ips"`
	RequiredScopes []string  `mapstructure:"required_scopes"  toml:"required_scopes"`
	ScopeMode      string    `mapstructure:"scope_mode"       toml:"scope_mode"`
	Description    string    `mapstructure:"description"      toml:"description"`
}

// ConcurrencyConfig 定义 HTTP/gRPC 并发限制配置。
type ConcurrencyConfig struct {
	HTTP struct {
		Enabled     bool          `mapstructure:"enabled"      toml:"enabled"`
		Max         int           `mapstructure:"max"          toml:"max"`
		WaitTimeout time.Duration `mapstructure:"wait_timeout" toml:"wait_timeout"`
	} `mapstructure:"http" toml:"http"`
	GRPC struct {
		Enabled     bool          `mapstructure:"enabled"      toml:"enabled"`
		Max         int           `mapstructure:"max"          toml:"max"`
		WaitTimeout time.Duration `mapstructure:"wait_timeout" toml:"wait_timeout"`
	} `mapstructure:"grpc" toml:"grpc"`
}

// GRPCKeepaliveConfig 定义 gRPC Keepalive 参数。
type GRPCKeepaliveConfig struct {
	Enabled               bool          `mapstructure:"enabled"                toml:"enabled"`
	Time                  time.Duration `mapstructure:"time"                   toml:"time"`
	Timeout               time.Duration `mapstructure:"timeout"                toml:"timeout"`
	PermitWithoutStream   bool          `mapstructure:"permit_without_stream"  toml:"permit_without_stream"`
	MinTime               time.Duration `mapstructure:"min_time"               toml:"min_time"`
	MaxConnectionIdle     time.Duration `mapstructure:"max_connection_idle"    toml:"max_connection_idle"`
	MaxConnectionAge      time.Duration `mapstructure:"max_connection_age"     toml:"max_connection_age"`
	MaxConnectionAgeGrace time.Duration `mapstructure:"max_connection_age_grace" toml:"max_connection_age_grace"`
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

// GetGRPCAddr 获取指定服务的 gRPC 地址.
func (c *Config) GetGRPCAddr(name string) string {
	if addr, ok := c.Services[name]; ok {
		return addr.GRPCAddr
	}
	return ""
}

// GetHTTPAddr 获取指定服务的 HTTP 地址.
func (c *Config) GetHTTPAddr(name string) string {
	if addr, ok := c.Services[name]; ok {
		return addr.HTTPAddr
	}
	return ""
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

// EtcdConfig 定义 Etcd 客户端连接参数.
type EtcdConfig struct {
	Endpoints   []string      `mapstructure:"endpoints"    toml:"endpoints"`
	Username    string        `mapstructure:"username"     toml:"username"`
	Password    string        `mapstructure:"password"     toml:"password"`
	DialTimeout time.Duration `mapstructure:"dial_timeout" toml:"dial_timeout"`
	AutoSync    time.Duration `mapstructure:"auto_sync"    toml:"auto_sync"`
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
var onReload []func(*Config)

// RegisterReloadHook 注册配置热更新回调。
func RegisterReloadHook(hook func(*Config)) {
	if hook == nil {
		return
	}
	onReload = append(onReload, hook)
}

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

		// 核心优化：如果配置中有日志级别，自动更新全局日志级别
		if c, ok := conf.(*Config); ok {
			logging.SetLevel(c.Log.Level)
		} else {
			// 尝试使用反射获取 Log.Level
			val := reflect.ValueOf(conf)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}
			logField := val.FieldByName("Log")
			if logField.IsValid() {
				levelField := logField.FieldByName("Level")
				if levelField.IsValid() && levelField.Kind() == reflect.String {
					logging.SetLevel(levelField.String())
				}
			}
		}

		if validateErr := validate.Struct(conf); validateErr != nil {
			slog.Error("reload config validation failed", "error", validateErr)
		} else {
			slog.Info("config hot-reloaded and validated successfully")
		}

		if cfg, ok := conf.(*Config); ok {
			for _, hook := range onReload {
				hook(cfg)
			}
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
