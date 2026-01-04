// Package cqrs 提供命令查询职责分离的基础设施和接口定义
package cqrs

import "context"

// Command 命令接口 marker interface
type Command interface {
	// CommandName 返回命令名称，用于路由和日志
	CommandName() string
}

// CommandHandler 命令处理器泛型接口
type CommandHandler[C Command] interface {
	// Handle 处理命令
	Handle(ctx context.Context, cmd C) error
}

// Query 查询接口 marker interface
type Query interface {
	// QueryName 返回查询名称
	QueryName() string
}

// QueryHandler 查询处理器泛型接口
type QueryHandler[Q Query, R any] interface {
	// Handle 处理查询并返回结果
	Handle(ctx context.Context, query Q) (R, error)
}

// CommandBus 命令总线接口
type CommandBus interface {
	// Dispatch 分发命令到对应的处理器
	Dispatch(ctx context.Context, cmd Command) error
	// Register 注册命令处理器
	// cmdName: 命令名称
	// handler: 处理器实例（需通过适配器转换为统一接口）
	Register(cmdName string, handler any)
}

// QueryBus 查询总线接口
type QueryBus interface {
	// Execute 执行查询并返回结果
	Execute(ctx context.Context, query Query) (any, error)
	// Register 注册查询处理器
	// queryName: 查询名称
	// handler: 处理器实例
	Register(queryName string, handler any)
}

// EventProcessor 事件处理器接口（用于 Read Model 投影）
type EventProcessor interface {
	// Subscribe 订阅特定类型的事件
	Subscribe(eventType string, handler EventHandler)
	// Handle 处理传入的事件
	Handle(ctx context.Context, event any) error
}

// EventHandler 事件处理函数类型
type EventHandler func(ctx context.Context, event any) error
