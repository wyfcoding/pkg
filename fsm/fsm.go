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

// State 定义状态标识符类型.
type State string

// Event 定义触发流转的事件标识符类型.
type Event string

// Handler 定义状态流转时执行的回调函数.
type Handler func(ctx context.Context, from, to State, args ...any) error

// Machine 封装了有限状态机的核心状态与流转逻辑.
type Machine struct {
	transitions map[State]map[Event]State
	handlers    map[State]map[State]Handler
	current     State
	mu          sync.RWMutex
}

// NewMachine 创建一个新的状态机.
func NewMachine(initial State) *Machine {
	return &Machine{
		current:     initial,
		transitions: make(map[State]map[Event]State),
		handlers:    make(map[State]map[State]Handler),
		mu:          sync.RWMutex{},
	}
}

// AddTransition 添加一条状态转移规则.
func (m *Machine) AddTransition(from State, event Event, to State) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.transitions[from]; !ok {
		m.transitions[from] = make(map[Event]State)
	}

	m.transitions[from][event] = to
}

// AddHandler 为特定的状态转移注册回调动作.
func (m *Machine) AddHandler(from, to State, handler Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.handlers[from]; !ok {
		m.handlers[from] = make(map[State]Handler)
	}

	m.handlers[from][to] = handler
}

// Current 获取状态机当前所处的状态.
func (m *Machine) Current() State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.current
}

// Trigger 触发一个事件.
func (m *Machine) Trigger(ctx context.Context, event Event, args ...any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	from := m.current
	to, ok := m.transitions[from][event]
	if !ok {
		return fmt.Errorf("%w: event %s for state %s", ErrInvalidTransition, event, from)
	}

	if handler, okH := m.handlers[from][to]; okH {
		if err := handler(ctx, from, to, args...); err != nil {
			return fmt.Errorf("%w (%s -> %s): %w", ErrHandlerFailed, from, to, err)
		}
	}

	m.current = to

	slog.InfoContext(ctx, "fsm state transitioned",
		"from", string(from),
		"to", string(to),
		"event", string(event),
	)

	return nil
}
