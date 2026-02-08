package httpclient

import (
	"sync"

	"github.com/wyfcoding/pkg/logging"
	"github.com/wyfcoding/pkg/metrics"
)

var (
	defaultClient *Client
	defaultOnce   sync.Once
)

// Default 返回全局默认 HTTP 客户端实例。
func Default() *Client {
	defaultOnce.Do(func() {
		if defaultClient == nil {
			defaultClient = NewClient(Config{ServiceName: "default"}, logging.Default(), metrics.NewMetrics("default"))
		}
	})

	return defaultClient
}

// SetDefault 设置全局默认 HTTP 客户端实例。
func SetDefault(c *Client) {
	if c == nil {
		return
	}
	defaultClient = c
}
