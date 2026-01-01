package fsm

import (
	"context"
	"fmt"
	"sync"
)

// State 状态类型
type State string

// Event 事件类型
type Event string

// Handler 状态转换处理函数
type Handler func(ctx context.Context, from State, to State, args ...interface{}) error

// Transition 状态转换定义
type Transition struct {
	From  State
	Event Event
	To    State
}

// Machine 有限状态机
type Machine struct {
	current     State
	transitions map[State]map[Event]State
	handlers    map[State]map[State]Handler
	mu          sync.RWMutex
}

// NewMachine 创建一个新的状态机
func NewMachine(initial State) *Machine {
	return &Machine{
		current:     initial,
		transitions: make(map[State]map[Event]State),
		handlers:    make(map[State]map[State]Handler),
	}
}

// AddTransition 添加转换规则
func (m *Machine) AddTransition(from State, event Event, to State) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.transitions[from]; !ok {
		m.transitions[from] = make(map[Event]State)
	}
	m.transitions[from][event] = to
}

// AddHandler 添加状态机转换执行动作
func (m *Machine) AddHandler(from State, to State, handler Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.handlers[from]; !ok {
		m.handlers[from] = make(map[State]Handler)
	}
	m.handlers[from][to] = handler
}

// Current 获取当前状态
func (m *Machine) Current() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Trigger 触发事件
func (m *Machine) Trigger(ctx context.Context, event Event, args ...interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	from := m.current
	to, ok := m.transitions[from][event]
	if !ok {
		return fmt.Errorf("invalid event %s for state %s", event, from)
	}

	// 执行 Handler (如果有)
	if handler, ok := m.handlers[from][to]; ok {
		if err := handler(ctx, from, to, args...); err != nil {
			return fmt.Errorf("handler failed for transition %s -> %s: %w", from, to, err)
		}
	}

	m.current = to
	return nil
}
