// Package fsm 提供通用的有限状态机 (Finite State Machine) 基础设施.
package fsm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

var (
	// ErrInvalidTransition 无效的状态转移.
	ErrInvalidTransition = errors.New("invalid state transition")
	// ErrHandlerFailed 处理器执行失败.
	ErrHandlerFailed = errors.New("fsm handler failed")
)

// Handler 定义状态流转时执行的回调函数.
type Handler[S comparable] func(ctx context.Context, from, to S, args ...any) error

// Machine 封装了有限状态机的核心状态与流转逻辑.
type Machine[S comparable, E comparable] struct {
	transitions map[S]map[E]S
	handlers    map[S]map[S]Handler[S]
	current     S
	mu          sync.RWMutex
}

// NewMachine 创建一个新的状态机.
func NewMachine[S comparable, E comparable](initial S) *Machine[S, E] {
	return &Machine[S, E]{
		current:     initial,
		transitions: make(map[S]map[E]S),
		handlers:    make(map[S]map[S]Handler[S]),
		mu:          sync.RWMutex{},
	}
}

// AddTransition 添加一条状态转移规则.
func (m *Machine[S, E]) AddTransition(from S, event E, to S) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.transitions[from]; !ok {
		m.transitions[from] = make(map[E]S)
	}

	m.transitions[from][event] = to
}

// AddHandler 为特定的状态转移注册回调动作.
func (m *Machine[S, E]) AddHandler(from, to S, handler Handler[S]) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.handlers[from]; !ok {
		m.handlers[from] = make(map[S]Handler[S])
	}

	m.handlers[from][to] = handler
}

// Current 获取状态机当前所处的状态.
func (m *Machine[S, E]) Current() S {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.current
}

// Trigger 触发一个事件.
func (m *Machine[S, E]) Trigger(ctx context.Context, event E, args ...any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	from := m.current
	to, ok := m.transitions[from][event]
	if !ok {
		return fmt.Errorf("%w: event %v for state %v", ErrInvalidTransition, event, from)
	}

	if handler, okH := m.handlers[from][to]; okH {
		if err := handler(ctx, from, to, args...); err != nil {
			return fmt.Errorf("%w (%v -> %v): %w", ErrHandlerFailed, from, to, err)
		}
	}

	m.current = to

	slog.InfoContext(ctx, "fsm state transitioned",
		"from", from,
		"to", to,
		"event", event,
	)

	return nil
}