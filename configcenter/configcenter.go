// Package configcenter 提供统一的配置中心客户端。
// 生成摘要：
// 1) 支持 Etcd/Consul/Nacos 等配置中心
// 2) 支持配置变更监听和热更新
// 3) 支持配置缓存和回退
// 假设：
// - 配置以 key-value 形式存储
// - 支持命名空间和分组
package configcenter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Provider 定义配置中心提供者类型。
type Provider string

const (
	// ProviderEtcd Etcd 配置中心。
	ProviderEtcd Provider = "etcd"
	// ProviderConsul Consul 配置中心。
	ProviderConsul Provider = "consul"
	// ProviderNacos Nacos 配置中心。
	ProviderNacos Provider = "nacos"
	// ProviderFile 文件配置（本地开发）。
	ProviderFile Provider = "file"
)

// Config 配置中心客户端配置。
type Config struct {
	// Provider 配置中心类型。
	Provider Provider `json:"provider" toml:"provider" mapstructure:"provider"`
	// Endpoints 服务端点列表。
	Endpoints []string `json:"endpoints" toml:"endpoints" mapstructure:"endpoints"`
	// Namespace 命名空间。
	Namespace string `json:"namespace" toml:"namespace" mapstructure:"namespace"`
	// Group 配置分组。
	Group string `json:"group" toml:"group" mapstructure:"group"`
	// Username 用户名。
	Username string `json:"username,omitempty" toml:"username" mapstructure:"username"`
	// Password 密码。
	Password string `json:"password,omitempty" toml:"password" mapstructure:"password"`
	// Timeout 请求超时。
	Timeout time.Duration `json:"timeout" toml:"timeout" mapstructure:"timeout"`
	// CacheDir 本地缓存目录。
	CacheDir string `json:"cache_dir,omitempty" toml:"cache_dir" mapstructure:"cache_dir"`
	// EnableCache 是否启用本地缓存。
	EnableCache bool `json:"enable_cache" toml:"enable_cache" mapstructure:"enable_cache"`
	// WatchInterval 轮询间隔（用于不支持 watch 的配置中心）。
	WatchInterval time.Duration `json:"watch_interval" toml:"watch_interval" mapstructure:"watch_interval"`
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *Config {
	return &Config{
		Provider:      ProviderEtcd,
		Endpoints:     []string{"localhost:2379"},
		Namespace:     "default",
		Group:         "default",
		Timeout:       5 * time.Second,
		EnableCache:   true,
		WatchInterval: 30 * time.Second,
	}
}

// ConfigItem 配置项。
type ConfigItem struct {
	// Key 配置键。
	Key string `json:"key"`
	// Value 配置值。
	Value string `json:"value"`
	// ContentType 内容类型（json/yaml/toml/text）。
	ContentType string `json:"content_type,omitempty"`
	// Version 版本号。
	Version int64 `json:"version"`
	// ModifiedAt 修改时间。
	ModifiedAt time.Time `json:"modified_at,omitempty"`
}

// ChangeEvent 配置变更事件。
type ChangeEvent struct {
	// Key 配置键。
	Key string `json:"key"`
	// OldValue 旧值。
	OldValue string `json:"old_value,omitempty"`
	// NewValue 新值。
	NewValue string `json:"new_value"`
	// EventType 事件类型。
	EventType EventType `json:"event_type"`
	// Timestamp 时间戳。
	Timestamp time.Time `json:"timestamp"`
}

// EventType 配置变更事件类型。
type EventType string

const (
	// EventTypeCreate 创建。
	EventTypeCreate EventType = "create"
	// EventTypeUpdate 更新。
	EventTypeUpdate EventType = "update"
	// EventTypeDelete 删除。
	EventTypeDelete EventType = "delete"
)

// ChangeHandler 配置变更处理器。
type ChangeHandler func(event *ChangeEvent)

// Client 配置中心客户端接口。
type Client interface {
	// Get 获取配置。
	Get(ctx context.Context, key string) (*ConfigItem, error)
	// GetWithDefault 获取配置，如果不存在则返回默认值。
	GetWithDefault(ctx context.Context, key, defaultValue string) string
	// GetJSON 获取 JSON 配置并反序列化。
	GetJSON(ctx context.Context, key string, target any) error
	// Set 设置配置。
	Set(ctx context.Context, key, value string) error
	// Delete 删除配置。
	Delete(ctx context.Context, key string) error
	// List 列出指定前缀的所有配置。
	List(ctx context.Context, prefix string) ([]*ConfigItem, error)
	// Watch 监听配置变更。
	Watch(ctx context.Context, key string, handler ChangeHandler) error
	// WatchPrefix 监听前缀下的所有配置变更。
	WatchPrefix(ctx context.Context, prefix string, handler ChangeHandler) error
	// Close 关闭客户端。
	Close() error
}

// EtcdClient Etcd 配置中心客户端实现。
type EtcdClient struct {
	client    *clientv3.Client
	config    *Config
	cache     map[string]*ConfigItem
	cacheMu   sync.RWMutex
	watchers  map[string]context.CancelFunc
	watcherMu sync.Mutex
	logger    *slog.Logger
}

// NewEtcdClient 创建 Etcd 客户端。
func NewEtcdClient(cfg *Config, logger *slog.Logger) (*EtcdClient, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	etcdCfg := clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: cfg.Timeout,
	}

	if cfg.Username != "" {
		etcdCfg.Username = cfg.Username
		etcdCfg.Password = cfg.Password
	}

	client, err := clientv3.New(etcdCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
	}

	return &EtcdClient{
		client:   client,
		config:   cfg,
		cache:    make(map[string]*ConfigItem),
		watchers: make(map[string]context.CancelFunc),
		logger:   logger,
	}, nil
}

// fullKey 构建完整的配置键。
func (c *EtcdClient) fullKey(key string) string {
	return fmt.Sprintf("/%s/%s/%s", c.config.Namespace, c.config.Group, key)
}

// Get 获取配置。
func (c *EtcdClient) Get(ctx context.Context, key string) (*ConfigItem, error) {
	fullKey := c.fullKey(key)

	// 尝试从缓存获取
	if c.config.EnableCache {
		c.cacheMu.RLock()
		if item, ok := c.cache[fullKey]; ok {
			c.cacheMu.RUnlock()
			return item, nil
		}
		c.cacheMu.RUnlock()
	}

	// 从 Etcd 获取
	resp, err := c.client.Get(ctx, fullKey)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to get config from etcd",
			"key", key,
			"error", err)
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("config not found: %s", key)
	}

	kv := resp.Kvs[0]
	item := &ConfigItem{
		Key:     key,
		Value:   string(kv.Value),
		Version: kv.ModRevision,
	}

	// 更新缓存
	if c.config.EnableCache {
		c.cacheMu.Lock()
		c.cache[fullKey] = item
		c.cacheMu.Unlock()
	}

	c.logger.DebugContext(ctx, "config retrieved",
		"key", key,
		"version", item.Version)

	return item, nil
}

// GetWithDefault 获取配置，如果不存在则返回默认值。
func (c *EtcdClient) GetWithDefault(ctx context.Context, key, defaultValue string) string {
	item, err := c.Get(ctx, key)
	if err != nil {
		return defaultValue
	}
	return item.Value
}

// GetJSON 获取 JSON 配置并反序列化。
func (c *EtcdClient) GetJSON(ctx context.Context, key string, target any) error {
	item, err := c.Get(ctx, key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(item.Value), target); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

// Set 设置配置。
func (c *EtcdClient) Set(ctx context.Context, key, value string) error {
	fullKey := c.fullKey(key)

	_, err := c.client.Put(ctx, fullKey, value)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to set config",
			"key", key,
			"error", err)
		return fmt.Errorf("failed to set config: %w", err)
	}

	// 更新缓存
	if c.config.EnableCache {
		c.cacheMu.Lock()
		c.cache[fullKey] = &ConfigItem{
			Key:        key,
			Value:      value,
			ModifiedAt: time.Now(),
		}
		c.cacheMu.Unlock()
	}

	c.logger.InfoContext(ctx, "config set successfully", "key", key)
	return nil
}

// Delete 删除配置。
func (c *EtcdClient) Delete(ctx context.Context, key string) error {
	fullKey := c.fullKey(key)

	_, err := c.client.Delete(ctx, fullKey)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to delete config",
			"key", key,
			"error", err)
		return fmt.Errorf("failed to delete config: %w", err)
	}

	// 删除缓存
	if c.config.EnableCache {
		c.cacheMu.Lock()
		delete(c.cache, fullKey)
		c.cacheMu.Unlock()
	}

	c.logger.InfoContext(ctx, "config deleted successfully", "key", key)
	return nil
}

// List 列出指定前缀的所有配置。
func (c *EtcdClient) List(ctx context.Context, prefix string) ([]*ConfigItem, error) {
	fullPrefix := c.fullKey(prefix)

	resp, err := c.client.Get(ctx, fullPrefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}

	items := make([]*ConfigItem, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		// 提取相对键名
		relKey := string(kv.Key)[len(c.fullKey("")):]
		items = append(items, &ConfigItem{
			Key:     relKey,
			Value:   string(kv.Value),
			Version: kv.ModRevision,
		})
	}

	return items, nil
}

// Watch 监听配置变更。
func (c *EtcdClient) Watch(ctx context.Context, key string, handler ChangeHandler) error {
	return c.WatchPrefix(ctx, key, handler)
}

// WatchPrefix 监听前缀下的所有配置变更。
func (c *EtcdClient) WatchPrefix(ctx context.Context, prefix string, handler ChangeHandler) error {
	fullPrefix := c.fullKey(prefix)

	// 创建可取消的上下文
	watchCtx, cancel := context.WithCancel(ctx)

	// 注册 watcher
	c.watcherMu.Lock()
	c.watchers[fullPrefix] = cancel
	c.watcherMu.Unlock()

	// 启动监听协程
	go func() {
		defer func() {
			c.watcherMu.Lock()
			delete(c.watchers, fullPrefix)
			c.watcherMu.Unlock()
		}()

		watchChan := c.client.Watch(watchCtx, fullPrefix, clientv3.WithPrefix())

		for {
			select {
			case <-watchCtx.Done():
				c.logger.Info("config watcher stopped", "prefix", prefix)
				return
			case watchResp := <-watchChan:
				if watchResp.Canceled {
					c.logger.Warn("config watch canceled", "prefix", prefix)
					return
				}

				for _, event := range watchResp.Events {
					relKey := string(event.Kv.Key)[len(c.fullKey("")):]

					changeEvent := &ChangeEvent{
						Key:       relKey,
						NewValue:  string(event.Kv.Value),
						Timestamp: time.Now(),
					}

					switch event.Type {
					case clientv3.EventTypePut:
						if event.IsCreate() {
							changeEvent.EventType = EventTypeCreate
						} else {
							changeEvent.EventType = EventTypeUpdate
							if event.PrevKv != nil {
								changeEvent.OldValue = string(event.PrevKv.Value)
							}
						}
					case clientv3.EventTypeDelete:
						changeEvent.EventType = EventTypeDelete
						if event.PrevKv != nil {
							changeEvent.OldValue = string(event.PrevKv.Value)
						}
					}

					// 更新缓存
					if c.config.EnableCache {
						c.cacheMu.Lock()
						if changeEvent.EventType == EventTypeDelete {
							delete(c.cache, string(event.Kv.Key))
						} else {
							c.cache[string(event.Kv.Key)] = &ConfigItem{
								Key:        relKey,
								Value:      changeEvent.NewValue,
								Version:    event.Kv.ModRevision,
								ModifiedAt: time.Now(),
							}
						}
						c.cacheMu.Unlock()
					}

					c.logger.InfoContext(ctx, "config changed",
						"key", relKey,
						"event_type", changeEvent.EventType)

					// 调用处理器
					handler(changeEvent)
				}
			}
		}
	}()

	return nil
}

// Close 关闭客户端。
func (c *EtcdClient) Close() error {
	// 停止所有 watchers
	c.watcherMu.Lock()
	for _, cancel := range c.watchers {
		cancel()
	}
	c.watchers = make(map[string]context.CancelFunc)
	c.watcherMu.Unlock()

	// 关闭 etcd 客户端
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("failed to close etcd client: %w", err)
	}

	c.logger.Info("config center client closed")
	return nil
}

// ConfigManager 配置管理器，提供更高级的配置管理功能。
type ConfigManager struct {
	client    Client
	cache     map[string]any
	cacheMu   sync.RWMutex
	handlers  map[string][]ChangeHandler
	handlerMu sync.RWMutex
	logger    *slog.Logger
}

// NewConfigManager 创建配置管理器。
func NewConfigManager(client Client, logger *slog.Logger) *ConfigManager {
	return &ConfigManager{
		client:   client,
		cache:    make(map[string]any),
		handlers: make(map[string][]ChangeHandler),
		logger:   logger,
	}
}

// Load 加载配置并反序列化到目标结构。
func (m *ConfigManager) Load(ctx context.Context, key string, target any) error {
	if err := m.client.GetJSON(ctx, key, target); err != nil {
		return err
	}

	// 缓存解析后的配置
	m.cacheMu.Lock()
	m.cache[key] = target
	m.cacheMu.Unlock()

	return nil
}

// Get 获取缓存的配置。
func (m *ConfigManager) Get(key string) (any, bool) {
	m.cacheMu.RLock()
	defer m.cacheMu.RUnlock()
	v, ok := m.cache[key]
	return v, ok
}

// OnChange 注册配置变更处理器。
func (m *ConfigManager) OnChange(key string, handler ChangeHandler) {
	m.handlerMu.Lock()
	m.handlers[key] = append(m.handlers[key], handler)
	m.handlerMu.Unlock()
}

// StartWatch 开始监听所有已注册的配置变更。
func (m *ConfigManager) StartWatch(ctx context.Context) error {
	m.handlerMu.RLock()
	keys := make([]string, 0, len(m.handlers))
	for key := range m.handlers {
		keys = append(keys, key)
	}
	m.handlerMu.RUnlock()

	for _, key := range keys {
		if err := m.client.Watch(ctx, key, func(event *ChangeEvent) {
			m.handlerMu.RLock()
			handlers := m.handlers[event.Key]
			m.handlerMu.RUnlock()

			for _, h := range handlers {
				h(event)
			}
		}); err != nil {
			return fmt.Errorf("failed to watch key %s: %w", key, err)
		}
	}

	return nil
}

// Close 关闭配置管理器。
func (m *ConfigManager) Close() error {
	return m.client.Close()
}

// NewClient 根据配置创建对应的客户端。
func NewClient(cfg *Config, logger *slog.Logger) (Client, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	switch cfg.Provider {
	case ProviderEtcd:
		return NewEtcdClient(cfg, logger)
	case ProviderConsul:
		return NewConsulClient(cfg, logger)
	case ProviderNacos:
		return NewNacosClient(cfg, logger)
	case ProviderFile:
		return NewFileClient(cfg, logger)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}
