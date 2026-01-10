package server

import "context"

// Server 接口定义了一个通用的服务器行为契约。
// 任何实现了 Start 和 Stop 方法的类型都可以被视为一个 Server.
// 从而实现服务器生命周期的统一管理。
type Server interface {
	// Start 方法用于启动服务器。
	// 它应该是一个阻塞调用，直到服务器准备好接收请求或上下文被取消。
	// ctx: 上下文，用于控制服务器的启动过程或传递取消信号。
	// 返回一个错误，表示服务器启动是否成功。
	Start(ctx context.Context) error
	// Stop 方法用于优雅地停止服务器。
	// 它应该等待正在处理的请求完成，并释放所有资源。
	// ctx: 上下文，用于控制服务器关闭过程的超时或传递取消信号。
	// 返回一个错误，表示服务器停止是否成功。
	Stop(ctx context.Context) error
}
