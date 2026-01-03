// Package app 提供了应用程序的构建和管理功能，包括服务的启动、停止和资源清理。
package app

import "github.com/wyfcoding/pkg/server" // 导入服务器接口定义

// Option 是一个函数类型，用于配置应用程序选项。
// 这种函数式选项模式（Functional Options Pattern）允许以可读和可扩展的方式配置结构体。
type Option func(*options)

// options 是应用程序的内部配置结构体。
// 它包含了应用程序运行时所需的所有可配置项。
type options struct {
	servers        []server.Server // 应用程序管理的服务器列表（例如，HTTP服务器、gRPC服务器）。
	cleanups       []func()        // 应用程序关闭时需要执行的清理函数列表（例如，关闭数据库连接、停止Kafka消费者）。
	healthCheckers []func() error  // 自定义健康检查探测器
}

// WithHealthChecker 注册一个自定义健康检查函数，用于服务的就绪状态检查。
func WithHealthChecker(checker func() error) Option {
	return func(o *options) {
		o.healthCheckers = append(o.healthCheckers, checker)
	}
}

// WithServer 是一个Option函数，用于向应用程序添加一个或多个 `server.Server` 实例。
// 这些服务器将在应用程序启动时被启动，并在应用程序关闭时被优雅地关闭。
func WithServer(servers ...server.Server) Option {
	return func(o *options) {
		o.servers = append(o.servers, servers...)
	}
}

// WithCleanup 是一个Option函数，用于向应用程序添加一个清理函数。
// 这些清理函数将在应用程序正常关闭时被执行，用于释放资源。
func WithCleanup(cleanup func()) Option {
	return func(o *options) {
		o.cleanups = append(o.cleanups, cleanup)
	}
}
