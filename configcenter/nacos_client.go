package configcenter

import (
	"context"
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

// NacosClient 提供基于 Nacos Config 的配置中心实现。
type NacosClient struct {
	config     *Config
	httpClient *http.Client
	logger     *slog.Logger
	cache      map[string]*ConfigItem
	cacheMu    sync.RWMutex
	endpointIx uint64
}

// NewNacosClient 创建 Nacos 配置中心客户端。
func NewNacosClient(cfg *Config, logger *slog.Logger) (*NacosClient, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if len(cfg.Endpoints) == 0 {
		return nil, errors.New("nacos endpoints is empty")
	}
	if logger == nil {
		logger = slog.Default()
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &NacosClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger:  logger,
		cache:   make(map[string]*ConfigItem),
		cacheMu: sync.RWMutex{},
	}, nil
}

func (c *NacosClient) pickEndpoint() string {
	idx := atomic.AddUint64(&c.endpointIx, 1)
	endpoint := c.config.Endpoints[int(idx)%len(c.config.Endpoints)]
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	return "http://" + endpoint
}

func (c *NacosClient) buildQuery(key string) url.Values {
	values := url.Values{}
	values.Set("dataId", key)
	if c.config.Group != "" {
		values.Set("group", c.config.Group)
	}
	if c.config.Namespace != "" {
		values.Set("tenant", c.config.Namespace)
	}
	if c.config.Username != "" {
		values.Set("username", c.config.Username)
	}
	if c.config.Password != "" {
		values.Set("password", c.config.Password)
	}
	return values
}

func (c *NacosClient) buildURL(path string, values url.Values) string {
	base := strings.TrimRight(c.pickEndpoint(), "/")
	if values == nil {
		return base + path
	}
	return base + path + "?" + values.Encode()
}

func (c *NacosClient) Get(ctx context.Context, key string) (*ConfigItem, error) {
	if c.config.EnableCache {
		c.cacheMu.RLock()
		if item, ok := c.cache[key]; ok {
			c.cacheMu.RUnlock()
			return item, nil
		}
		c.cacheMu.RUnlock()
	}

	reqURL := c.buildURL("/nacos/v1/cs/configs", c.buildQuery(key))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create nacos request: %w", err)
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
		return nil, fmt.Errorf("nacos response error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read nacos response: %w", err)
	}

	item := &ConfigItem{
		Key:   key,
		Value: string(body),
	}

	if c.config.EnableCache {
		c.cacheMu.Lock()
		c.cache[key] = item
		c.cacheMu.Unlock()
	}

	return item, nil
}

func (c *NacosClient) GetWithDefault(ctx context.Context, key, defaultValue string) string {
	item, err := c.Get(ctx, key)
	if err != nil {
		return defaultValue
	}
	return item.Value
}

func (c *NacosClient) GetJSON(ctx context.Context, key string, target any) error {
	item, err := c.Get(ctx, key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(item.Value), target); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return nil
}

func (c *NacosClient) Set(ctx context.Context, key, value string) error {
	form := c.buildQuery(key)
	form.Set("content", value)

	reqURL := c.buildURL("/nacos/v1/cs/configs", nil)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create nacos request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to set config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("nacos set failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	if c.config.EnableCache {
		c.cacheMu.Lock()
		c.cache[key] = &ConfigItem{
			Key:        key,
			Value:      value,
			ModifiedAt: time.Now(),
		}
		c.cacheMu.Unlock()
	}

	return nil
}

func (c *NacosClient) Delete(ctx context.Context, key string) error {
	reqURL := c.buildURL("/nacos/v1/cs/configs", c.buildQuery(key))
	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create nacos delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("nacos delete failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	if c.config.EnableCache {
		c.cacheMu.Lock()
		delete(c.cache, key)
		c.cacheMu.Unlock()
	}

	return nil
}

func (c *NacosClient) List(ctx context.Context, prefix string) ([]*ConfigItem, error) {
	return nil, errors.New("nacos list is not supported")
}

func (c *NacosClient) Watch(ctx context.Context, key string, handler ChangeHandler) error {
	interval := c.config.WatchInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	go func() {
		var previous string
		initialized := false
		if item, err := c.Get(ctx, key); err == nil {
			previous = item.Value
			initialized = true
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				item, err := c.Get(ctx, key)
				if err != nil {
					c.logger.Warn("nacos watch get failed", "error", err)
					continue
				}
				if !initialized {
					previous = item.Value
					initialized = true
					continue
				}
				if item.Value != previous {
					handler(&ChangeEvent{
						Key:       key,
						OldValue:  previous,
						NewValue:  item.Value,
						EventType: EventTypeUpdate,
						Timestamp: time.Now(),
					})
					previous = item.Value
				}
			}
		}
	}()

	return nil
}

func (c *NacosClient) WatchPrefix(ctx context.Context, prefix string, handler ChangeHandler) error {
	return errors.New("nacos watch prefix is not supported")
}

func (c *NacosClient) Close() error {
	c.logger.Info("nacos config center client closed")
	return nil
}
