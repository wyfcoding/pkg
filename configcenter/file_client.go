package configcenter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileClient 提供基于本地文件的配置中心实现。
type FileClient struct {
	config  *Config
	logger  *slog.Logger
	cache   map[string]*ConfigItem
	cacheMu sync.RWMutex
}

// NewFileClient 创建文件配置中心客户端。
func NewFileClient(cfg *Config, logger *slog.Logger) (*FileClient, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &FileClient{
		config: cfg,
		logger: logger,
		cache:  make(map[string]*ConfigItem),
	}, nil
}

func (c *FileClient) baseDir() string {
	if c.config.CacheDir != "" {
		return c.config.CacheDir
	}
	return "."
}

func (c *FileClient) fullPath(key string) string {
	trimmed := strings.TrimPrefix(key, string(filepath.Separator))
	if c.config.Namespace == "" && c.config.Group == "" {
		return filepath.Join(c.baseDir(), trimmed)
	}
	return filepath.Join(c.baseDir(), c.config.Namespace, c.config.Group, trimmed)
}

func (c *FileClient) Get(ctx context.Context, key string) (*ConfigItem, error) {
	fullKey := c.fullPath(key)
	if c.config.EnableCache {
		c.cacheMu.RLock()
		if item, ok := c.cache[fullKey]; ok {
			c.cacheMu.RUnlock()
			return item, nil
		}
		c.cacheMu.RUnlock()
	}

	data, err := os.ReadFile(fullKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	info, err := os.Stat(fullKey)
	if err != nil {
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}

	item := &ConfigItem{
		Key:        key,
		Value:      string(data),
		Version:    info.ModTime().UnixNano(),
		ModifiedAt: info.ModTime(),
	}

	if c.config.EnableCache {
		c.cacheMu.Lock()
		c.cache[fullKey] = item
		c.cacheMu.Unlock()
	}

	return item, nil
}

func (c *FileClient) GetWithDefault(ctx context.Context, key, defaultValue string) string {
	item, err := c.Get(ctx, key)
	if err != nil {
		return defaultValue
	}
	return item.Value
}

func (c *FileClient) GetJSON(ctx context.Context, key string, target any) error {
	item, err := c.Get(ctx, key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(item.Value), target); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}

func (c *FileClient) Set(ctx context.Context, key, value string) error {
	fullKey := c.fullPath(key)
	if err := os.MkdirAll(filepath.Dir(fullKey), 0o755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	if err := os.WriteFile(fullKey, []byte(value), 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	info, err := os.Stat(fullKey)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	if c.config.EnableCache {
		c.cacheMu.Lock()
		c.cache[fullKey] = &ConfigItem{
			Key:        key,
			Value:      value,
			Version:    info.ModTime().UnixNano(),
			ModifiedAt: info.ModTime(),
		}
		c.cacheMu.Unlock()
	}

	return nil
}

func (c *FileClient) Delete(ctx context.Context, key string) error {
	fullKey := c.fullPath(key)
	if err := os.Remove(fullKey); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to delete config file: %w", err)
	}

	if c.config.EnableCache {
		c.cacheMu.Lock()
		delete(c.cache, fullKey)
		c.cacheMu.Unlock()
	}

	return nil
}

func (c *FileClient) List(ctx context.Context, prefix string) ([]*ConfigItem, error) {
	root := c.fullPath(prefix)
	items := make([]*ConfigItem, 0)

	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return items, nil
		}
		return nil, err
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(c.fullPath(""), path)
		if err != nil {
			return err
		}

		items = append(items, &ConfigItem{
			Key:        filepath.ToSlash(rel),
			Value:      string(data),
			Version:    info.ModTime().UnixNano(),
			ModifiedAt: info.ModTime(),
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	return items, nil
}

func (c *FileClient) Watch(ctx context.Context, key string, handler ChangeHandler) error {
	return c.WatchPrefix(ctx, key, handler)
}

func (c *FileClient) WatchPrefix(ctx context.Context, prefix string, handler ChangeHandler) error {
	interval := c.config.WatchInterval
	if interval <= 0 {
		interval = 10 * time.Second
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
					c.logger.Warn("file watch list failed", "error", err)
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

func (c *FileClient) Close() error {
	c.logger.Info("file config center client closed")
	return nil
}
