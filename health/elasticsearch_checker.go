package health

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
)

const defaultElasticsearchHealthTimeout = 2 * time.Second

// ElasticsearchChecker 返回 Elasticsearch 依赖健康检查函数。
func ElasticsearchChecker(cfg elasticsearch.Config, timeout time.Duration) Checker {
	return func() error {
		if len(cfg.Addresses) == 0 {
			return errors.New("elasticsearch addresses is empty")
		}
		if timeout <= 0 {
			timeout = defaultElasticsearchHealthTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		client, err := elasticsearch.NewClient(cfg)
		if err != nil {
			return fmt.Errorf("elasticsearch client init failed: %w", err)
		}

		resp, err := client.Cluster.Health(client.Cluster.Health.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("elasticsearch health check failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.IsError() {
			return fmt.Errorf("elasticsearch health status code: %s", resp.Status())
		}

		return nil
	}
}

// ElasticsearchCheckerFromAddresses 返回基于地址列表的健康检查函数。
func ElasticsearchCheckerFromAddresses(addresses []string, timeout time.Duration) Checker {
	cfg := elasticsearch.Config{
		Addresses: addresses,
	}
	return ElasticsearchChecker(cfg, timeout)
}

// ElasticsearchCheckerFromURL 返回基于单个地址的健康检查函数。
func ElasticsearchCheckerFromURL(addr string, timeout time.Duration) Checker {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return func() error {
			return errors.New("elasticsearch address is empty")
		}
	}
	return ElasticsearchCheckerFromAddresses([]string{addr}, timeout)
}
