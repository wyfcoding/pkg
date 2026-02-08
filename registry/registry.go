package registry

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrInvalidInstance 服务实例信息非法。
	ErrInvalidInstance = errors.New("invalid service instance")
	// ErrHandlerNil 事件处理函数为空。
	ErrHandlerNil = errors.New("watch handler is nil")
)

// ServiceInstance 表示一个可被发现的服务实例。
type ServiceInstance struct {
	ID        string            `json:"id"`         // 实例唯一 ID。
	Name      string            `json:"name"`       // 服务名称。
	Address   string            `json:"address"`    // 访问地址 (host:port)。
	Tags      []string          `json:"tags"`       // 实例标签。
	Metadata  map[string]string `json:"metadata"`   // 扩展元数据。
	Weight    int               `json:"weight"`     // 权重。
	UpdatedAt time.Time         `json:"updated_at"` // 更新时间。
}

// WatchHandler 定义注册中心的变更处理函数。
type WatchHandler func(instances []ServiceInstance)

// Registry 定义服务注册发现的统一接口。
type Registry interface {
	// Register 注册服务实例，返回停止续租的函数。
	Register(ctx context.Context, instance ServiceInstance) (func(), error)
	// Deregister 注销服务实例。
	Deregister(ctx context.Context, instance ServiceInstance) error
	// GetService 获取服务的所有实例。
	GetService(ctx context.Context, serviceName string) ([]ServiceInstance, error)
	// Watch 监听指定服务的实例变化。
	Watch(ctx context.Context, serviceName string, handler WatchHandler) error
}

var (
	defaultRegistry Registry
	registryMu      sync.RWMutex
)

// Default 返回默认注册中心实例。
func Default() Registry {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return defaultRegistry
}

// SetDefault 设置默认注册中心实例。
func SetDefault(reg Registry) {
	if reg == nil {
		return
	}
	registryMu.Lock()
	defaultRegistry = reg
	registryMu.Unlock()
}
