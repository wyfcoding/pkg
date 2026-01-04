package cqrs

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

// InMemCommandBus 高性能内存命令总线实现
// 优化：在注册时利用反射生成包装函数，分发时直接调用，避免运行时的反射查找开销
type InMemCommandBus struct {
	handlers map[string]func(context.Context, Command) error
	mu       sync.RWMutex
}

// NewInMemCommandBus 创建一个新的 InMemCommandBus
func NewInMemCommandBus() *InMemCommandBus {
	return &InMemCommandBus{
		handlers: make(map[string]func(context.Context, Command) error),
	}
}

// Register 注册命令处理器
// handler 必须实现 CommandHandler[C] 接口
func (b *InMemCommandBus) Register(cmdName string, handler any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlerValue := reflect.ValueOf(handler)
	method := handlerValue.MethodByName("Handle")

	if !method.IsValid() {
		panic(fmt.Sprintf("handler for command %s does not have a Handle method", cmdName))
	}

	// 检查 Handle 方法签名: func(ctx context.Context, cmd C) error
	methodType := method.Type()
	if methodType.NumIn() != 2 || methodType.NumOut() != 1 {
		panic(fmt.Sprintf("handler for command %s has invalid signature, expected 2 args and 1 return", cmdName))
	}

	// 生成包装函数闭包
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

// Dispatch 分发命令
func (b *InMemCommandBus) Dispatch(ctx context.Context, cmd Command) error {
	b.mu.RLock()
	handler, ok := b.handlers[cmd.CommandName()]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no handler registered for command: %s", cmd.CommandName())
	}

	return handler(ctx, cmd)
}

// InMemQueryBus 高性能内存查询总线实现
type InMemQueryBus struct {
	handlers map[string]func(context.Context, Query) (any, error)
	mu       sync.RWMutex
}

// NewInMemQueryBus 创建一个新的 InMemQueryBus
func NewInMemQueryBus() *InMemQueryBus {
	return &InMemQueryBus{
		handlers: make(map[string]func(context.Context, Query) (any, error)),
	}
}

// Register 注册查询处理器
// handler 必须实现 QueryHandler[Q, R] 接口
func (b *InMemQueryBus) Register(queryName string, handler any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlerValue := reflect.ValueOf(handler)
	method := handlerValue.MethodByName("Handle")

	if !method.IsValid() {
		panic(fmt.Sprintf("handler for query %s does not have a Handle method", queryName))
	}

	// 检查 Handle 方法签名: func(ctx context.Context, query Q) (R, error)
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

// Execute 执行查询
func (b *InMemQueryBus) Execute(ctx context.Context, query Query) (any, error) {
	b.mu.RLock()
	handler, ok := b.handlers[query.QueryName()]
	b.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no handler registered for query: %s", query.QueryName())
	}

	return handler(ctx, query)
}
