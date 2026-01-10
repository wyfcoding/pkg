// Package app 提供了应用程序生命周期管理的基础设施。
package app

import (
	"github.com/wyfcoding/pkg/server" // 导入服务器接口定义。
)

type options struct {
	servers        []server.Server
	cleanups       []func()
	healthCheckers []func() error // 自定义健康检查探测器列表。
}

// Option 定义配置 App 的函数原型。
type Option func(*options)

// WithServer 注册一个或多个服务器实例。
func WithServer(servers ...server.Server) Option {
	return func(o *options) {
		o.servers = append(o.servers, servers...)
	}
}

// WithCleanup 注册一个资源清理函数。
func WithCleanup(cleanup func()) Option {
	return func(o *options) {
		o.cleanups = append(o.cleanups, cleanup)
	}
}

// WithHealthChecker 注册一个健康检查探测器。
func WithHealthChecker(checker func() error) Option {
	return func(o *options) {
		o.healthCheckers = append(o.healthCheckers, checker)
	}
}
