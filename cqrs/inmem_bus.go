package cqrs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"
)

var (
	// ErrNoCommandHandler 未注册命令处理器.
	ErrNoCommandHandler = errors.New("no handler registered for command")
	// ErrNoQueryHandler 未注册查询处理器.
	ErrNoQueryHandler = errors.New("no handler registered for query")
)

// InMemCommandBus 高性能内存命令总线实现.
type InMemCommandBus struct { //nolint:govet
	handlers map[string]func(context.Context, Command) error
	mu       sync.RWMutex
}

// NewInMemCommandBus 创建一个新的 InMemCommandBus 实例.
func NewInMemCommandBus() *InMemCommandBus {
	return &InMemCommandBus{
		handlers: make(map[string]func(context.Context, Command) error),
		mu:       sync.RWMutex{},
	}
}

// Register 注册命令处理器.
func (b *InMemCommandBus) Register(cmdName string, handler any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlerValue := reflect.ValueOf(handler)
	method := handlerValue.MethodByName("Handle")

	if !method.IsValid() {
		panic(fmt.Sprintf("handler for command %s does not have a Handle method", cmdName))
	}

	methodType := method.Type()
	if methodType.NumIn() != 2 || methodType.NumOut() != 1 {
		panic(fmt.Sprintf("handler for command %s has invalid signature", cmdName))
	}

	wrapper := func(ctx context.Context, cmd Command) error {
		args := []reflect.Value{
			reflect.ValueOf(ctx),
			reflect.ValueOf(cmd),
		}

		results := method.Call(args)

		if !results[0].IsNil() {
			if err, ok := results[0].Interface().(error); ok {
				return err
			}
		}

		return nil
	}

	b.handlers[cmdName] = wrapper
}

// Dispatch 将命令分发至已注册的处理器.
func (b *InMemCommandBus) Dispatch(ctx context.Context, cmd Command) error {
	cmdName := cmd.CommandName()
	start := time.Now()

	b.mu.RLock()
	handler, ok := b.handlers[cmdName]
	b.mu.RUnlock()

	if !ok {
		slog.ErrorContext(ctx, "no command handler registered", "command", cmdName)

		return fmt.Errorf("%w: %s", ErrNoCommandHandler, cmdName)
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

// InMemQueryBus 高性能内存查询总线实现.
type InMemQueryBus struct { //nolint:govet
	handlers map[string]func(context.Context, Query) (any, error)
	mu       sync.RWMutex
}

// NewInMemQueryBus 创建一个新的 InMemQueryBus 实例.
func NewInMemQueryBus() *InMemQueryBus {
	return &InMemQueryBus{
		handlers: make(map[string]func(context.Context, Query) (any, error)),
		mu:       sync.RWMutex{},
	}
}

// Register 注册查询处理器.
func (b *InMemQueryBus) Register(queryName string, handler any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlerValue := reflect.ValueOf(handler)
	method := handlerValue.MethodByName("Handle")

	if !method.IsValid() {
		panic(fmt.Sprintf("handler for query %s does not have a Handle method", queryName))
	}

	methodType := method.Type()
	if methodType.NumIn() != 2 || methodType.NumOut() != 2 {
		panic(fmt.Sprintf("handler for query %s has invalid signature", queryName))
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
			if e, ok := results[1].Interface().(error); ok {
				err = e
			}
		}

		return res, err
	}

	b.handlers[queryName] = wrapper
}

// Execute 执行查询并返回结果.
func (b *InMemQueryBus) Execute(ctx context.Context, query Query) (any, error) {
	queryName := query.QueryName()
	start := time.Now()

	b.mu.RLock()
	handler, ok := b.handlers[queryName]
	b.mu.RUnlock()

	if !ok {
		slog.ErrorContext(ctx, "no query handler registered", "query", queryName)

		return nil, fmt.Errorf("%w: %s", ErrNoQueryHandler, queryName)
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
