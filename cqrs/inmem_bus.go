package cqrs

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"
)

// InMemCommandBus 高性能内存命令总线实现。
// 优化策略：在注册阶段利用反射生成高效包装闭包，分发阶段实现 O(1) 直接调用，避免运行时反复反射损耗。
type InMemCommandBus struct {
	handlers map[string]func(context.Context, Command) error
	mu       sync.RWMutex
}

// NewInMemCommandBus 创建一个新的 InMemCommandBus 实例。
func NewInMemCommandBus() *InMemCommandBus {
	return &InMemCommandBus{
		handlers: make(map[string]func(context.Context, Command) error),
	}
}

// Register 注册命令处理器。
// handler 必须实现 CommandHandler[C] 接口。
func (b *InMemCommandBus) Register(cmdName string, handler any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlerValue := reflect.ValueOf(handler)
	method := handlerValue.MethodByName("Handle")

	if !method.IsValid() {
		panic(fmt.Sprintf("handler for command %s does not have a Handle method", cmdName))
	}

	// 检查 Handle 方法签名: func(ctx context.Context, cmd C) error。
	methodType := method.Type()
	if methodType.NumIn() != 2 || methodType.NumOut() != 1 {
		panic(fmt.Sprintf("handler for command %s has invalid signature, expected 2 args and 1 return", cmdName))
	}

	// 生成包装函数闭包。
	wrapper := func(ctx context.Context, cmd Command) error {
		args := []reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(cmd),
		}

		results := method.Call(args)

		if !results[0].IsNil() {
			return results[0].Interface().(error)
		}

		return nil
	}

	b.handlers[cmdName] = wrapper
}

// Dispatch 将命令分发至已注册的处理器。
// 关键业务逻辑：输出分发耗时与状态日志。
func (b *InMemCommandBus) Dispatch(ctx context.Context, cmd Command) error {
	cmdName := cmd.CommandName()
	start := time.Now()

	b.mu.RLock()
	handler, ok := b.handlers[cmdName]
	b.mu.RUnlock()

	if !ok {
		slog.ErrorContext(ctx, "no command handler registered", "command", cmdName)

		return fmt.Errorf("no handler registered for command: %s", cmdName)
	}

	err := handler(ctx, cmd)
	duration := time.Since(start)

	if err != nil {
		slog.ErrorContext(ctx, "command dispatch failed", "command", cmdName, "error", err, "duration", duration)
	} else {
		slog.InfoContext(ctx, "command dispatched successfully", "command", cmdName, "duration", duration)
	}

	return err
}

// InMemQueryBus 高性能内存查询总线实现。
type InMemQueryBus struct {
	handlers map[string]func(context.Context, Query) (any, error)
	mu       sync.RWMutex
}

// NewInMemQueryBus 创建一个新的 InMemQueryBus 实例。
func NewInMemQueryBus() *InMemQueryBus {
	return &InMemQueryBus{
		handlers: make(map[string]func(context.Context, Query) (any, error)),
	}
}

// Register 注册查询处理器。
// handler 必须实现 QueryHandler[Q, R] 接口。
func (b *InMemQueryBus) Register(queryName string, handler any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlerValue := reflect.ValueOf(handler)
	method := handlerValue.MethodByName("Handle")

	if !method.IsValid() {
		panic(fmt.Sprintf("handler for query %s does not have a Handle method", queryName))
	}

	// 检查 Handle 方法签名: func(ctx context.Context, query Q) (R, error)。
	methodType := method.Type()
	if methodType.NumIn() != 2 || methodType.NumOut() != 2 {
		panic(fmt.Sprintf("handler for query %s has invalid signature, expected 2 args and 2 returns", queryName))
	}

	wrapper := func(ctx context.Context, query Query) (any, error) {
		args := []reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(query),
		}

		results := method.Call(args)

		var res any
		var err error

		if len(results) > 0 {
			res = results[0].Interface()
		}

		if len(results) > 1 && !results[1].IsNil() {
			err = results[1].Interface().(error)
		}

		return res, err
	}

	b.handlers[queryName] = wrapper
}

// Execute 执行查询并返回结果。
// 关键业务逻辑：记录查询执行情况。
func (b *InMemQueryBus) Execute(ctx context.Context, query Query) (any, error) {
	queryName := query.QueryName()
	start := time.Now()

	b.mu.RLock()
	handler, ok := b.handlers[queryName]
	b.mu.RUnlock()

	if !ok {
		slog.ErrorContext(ctx, "no query handler registered", "query", queryName)
		return nil, fmt.Errorf("no handler registered for query: %s", queryName)
	}

	res, err := handler(ctx, query)
	duration := time.Since(start)

	if err != nil {
		slog.ErrorContext(ctx, "query execution failed", "query", queryName, "error", err, "duration", duration)
	} else {
		slog.DebugContext(ctx, "query executed successfully", "query", queryName, "duration", duration)
	}

	return res, err
}
