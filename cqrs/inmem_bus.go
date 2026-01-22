package cqrs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
type InMemCommandBus struct {
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

// RegisterCommand 注册命令处理器.
func RegisterCommand[C Command](bus *InMemCommandBus, handler CommandHandler[C]) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	var cmd C
	cmdName := cmd.CommandName()

	bus.handlers[cmdName] = func(ctx context.Context, c Command) error {
		return handler.Handle(ctx, c.(C))
	}
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
type InMemQueryBus struct {
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

// RegisterQuery 注册查询处理器.
func RegisterQuery[Q Query, R any](bus *InMemQueryBus, handler QueryHandler[Q, R]) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	var q Q
	queryName := q.QueryName()

	bus.handlers[queryName] = func(ctx context.Context, query Query) (any, error) {
		return handler.Handle(ctx, query.(Q))
	}
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
