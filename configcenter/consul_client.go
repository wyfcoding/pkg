package configcenter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ConsulClient 提供基于 Consul KV 的配置中心实现。
type ConsulClient struct {
	config     *Config
	httpClient *http.Client
	logger     *slog.Logger
	cache      map[string]*ConfigItem
	cacheMu    sync.RWMutex
	endpointIx uint64
}

// NewConsulClient 创建 Consul 配置中心客户端。
func NewConsulClient(cfg *Config, logger *slog.Logger) (*ConsulClient, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if len(cfg.Endpoints) == 0 {
		return nil, errors.New("consul endpoints is empty")
	}
	if logger == nil {
		logger = slog.Default()
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &ConsulClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger:  logger,
		cache:   make(map[string]*ConfigItem),
		cacheMu: sync.RWMutex{},
	}, nil
}

func (c *ConsulClient) fullKey(key string) string {
	trimmed := strings.TrimPrefix(key, "/")
	if c.config.Namespace == "" && c.config.Group == "" {
		return trimmed
	}
	return strings.TrimPrefix(fmt.Sprintf("/%s/%s/%s", c.config.Namespace, c.config.Group, trimmed), "/")
}

func (c *ConsulClient) pickEndpoint() string {
	idx := atomic.AddUint64(&c.endpointIx, 1)
	endpoint := c.config.Endpoints[int(idx)%len(c.config.Endpoints)]
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	return "http://" + endpoint
}

func (c *ConsulClient) buildURL(path string, query url.Values) string {
	base := strings.TrimRight(c.pickEndpoint(), "/")
	if query == nil || len(query) == 0 {
		return base + path
	}
	return base + path + "?" + query.Encode()
}

func (c *ConsulClient) escapeKeyPath(key string) string {
	if key == "" {
		return ""
	}
	segments := strings.Split(strings.TrimPrefix(key, "/"), "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

func (c *ConsulClient) Get(ctx context.Context, key string) (*ConfigItem, error) {
	fullKey := c.fullKey(key)
	if c.config.EnableCache {
		c.cacheMu.RLock()
		if item, ok := c.cache[fullKey]; ok {
			c.cacheMu.RUnlock()
			return item, nil
		}
		c.cacheMu.RUnlock()
	}

	items, err := c.getKV(ctx, fullKey, false)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("config not found: %s", key)
	}

	item := items[0]
	item.Key = key
	if c.config.EnableCache {
		c.cacheMu.Lock()
		c.cache[fullKey] = item
		c.cacheMu.Unlock()
	}

	return item, nil
}

func (c *ConsulClient) GetWithDefault(ctx context.Context, key, defaultValue string) string {
	item, err := c.Get(ctx, key)
	if err != nil {
		return defaultValue
	}
	return item.Value
}

func (c *ConsulClient) GetJSON(ctx context.Context, key string, target any) error {
	item, err := c.Get(ctx, key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(item.Value), target); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

func (c *ConsulClient) Set(ctx context.Context, key, value string) error {
	fullKey := c.fullKey(key)
	reqURL := c.buildURL("/v1/kv/"+c.escapeKeyPath(fullKey), nil)

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, strings.NewReader(value))
	if err != nil {
		return fmt.Errorf("failed to create consul request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to set config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("consul set failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	if c.config.EnableCache {
		c.cacheMu.Lock()
		c.cache[fullKey] = &ConfigItem{
			Key:        key,
			Value:      value,
			ModifiedAt: time.Now(),
		}
		c.cacheMu.Unlock()
	}

	return nil
}

func (c *ConsulClient) Delete(ctx context.Context, key string) error {
	fullKey := c.fullKey(key)
	reqURL := c.buildURL("/v1/kv/"+c.escapeKeyPath(fullKey), nil)

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create consul delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("consul delete failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	if c.config.EnableCache {
		c.cacheMu.Lock()
		delete(c.cache, fullKey)
		c.cacheMu.Unlock()
	}

	return nil
}

func (c *ConsulClient) List(ctx context.Context, prefix string) ([]*ConfigItem, error) {
	fullPrefix := c.fullKey(prefix)
	items, err := c.getKV(ctx, fullPrefix, true)
	if err != nil {
		return nil, err
	}

	root := strings.TrimPrefix(c.fullKey(""), "/")
	for _, item := range items {
		if root != "" && strings.HasPrefix(item.Key, root) {
			item.Key = strings.TrimPrefix(item.Key, root)
			item.Key = strings.TrimPrefix(item.Key, "/")
		}
		item.Key = strings.TrimPrefix(item.Key, "/")
	}

	return items, nil
}

func (c *ConsulClient) Watch(ctx context.Context, key string, handler ChangeHandler) error {
	return c.WatchPrefix(ctx, key, handler)
}

func (c *ConsulClient) WatchPrefix(ctx context.Context, prefix string, handler ChangeHandler) error {
	interval := c.config.WatchInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	go func() {
		prev := make(map[string]string)
		if items, err := c.List(ctx, prefix); err == nil {
			for _, item := range items {
				prev[item.Key] = item.Value
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				items, err := c.List(ctx, prefix)
				if err != nil {
					c.logger.Warn("consul watch list failed", "error", err)
					continue
				}

				curr := make(map[string]string, len(items))
				for _, item := range items {
					curr[item.Key] = item.Value
				}

				for key, value := range curr {
					if old, ok := prev[key]; !ok {
						handler(&ChangeEvent{
							Key:       key,
							NewValue:  value,
							EventType: EventTypeCreate,
							Timestamp: time.Now(),
						})
					} else if old != value {
						handler(&ChangeEvent{
							Key:       key,
							OldValue:  old,
							NewValue:  value,
							EventType: EventTypeUpdate,
							Timestamp: time.Now(),
						})
					}
				}

				for key, value := range prev {
					if _, ok := curr[key]; !ok {
						handler(&ChangeEvent{
							Key:       key,
							OldValue:  value,
							EventType: EventTypeDelete,
							Timestamp: time.Now(),
						})
					}
				}

				prev = curr
			}
		}
	}()

	return nil
}

func (c *ConsulClient) Close() error {
	c.logger.Info("consul config center client closed")
	return nil
}

type consulKVItem struct {
	Key         string `json:"Key"`
	Value       string `json:"Value"`
	ModifyIndex uint64 `json:"ModifyIndex"`
}

func (c *ConsulClient) getKV(ctx context.Context, key string, recurse bool) ([]*ConfigItem, error) {
	query := url.Values{}
	if recurse {
		query.Set("recurse", "true")
	}

	reqURL := c.buildURL("/v1/kv/"+c.escapeKeyPath(key), query)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("config not found: %s", key)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("consul response error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read consul response: %w", err)
	}

	var kvs []consulKVItem
	if err := json.Unmarshal(body, &kvs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal consul response: %w", err)
	}

	items := make([]*ConfigItem, 0, len(kvs))
	for _, kv := range kvs {
		decoded, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to decode consul value: %w", err)
		}
		items = append(items, &ConfigItem{
			Key:     kv.Key,
			Value:   string(decoded),
			Version: int64(kv.ModifyIndex),
		})
	}

	return items, nil
}
