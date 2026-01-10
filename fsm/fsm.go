// Package fsm 提供通用的有限状态机（Finite State Machine）基础设施，支持基于事件驱动的状态流转。
package fsm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// State 定义状态标识符类型。
type State string

// Event 定义触发流转的事件标识符类型。
type Event string

// Handler 定义状态流转时执行的回调函数。
type Handler func(ctx context.Context, from State, to State, args ...any) error

// Machine 封装了有限状态机的核心状态与流转逻辑。
type Machine struct {
	current     State                       // 当前所处的状态。
	transitions map[State]map[Event]State   // 注册的状态转移矩阵。
	handlers    map[State]map[State]Handler // 注册的状态转移执行动作。
	mu          sync.RWMutex                // 保证并发安全。
}

// NewMachine 创建一个新的状态机。
func NewMachine(initial State) *Machine {
	return &Machine{
		current:     initial,
		transitions: make(map[State]map[Event]State),
		handlers:    make(map[State]map[State]Handler),
	}
}

// AddTransition 添加一条状态转移规则。
func (m *Machine) AddTransition(from State, event Event, to State) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.transitions[from]; !ok {
		m.transitions[from] = make(map[Event]State)
	}
	m.transitions[from][event] = to
}

// AddHandler 为特定的状态转移注册回调动作。
func (m *Machine) AddHandler(from State, to State, handler Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.handlers[from]; !ok {
		m.handlers[from] = make(map[State]Handler)
	}
	m.handlers[from][to] = handler
}

// Current 获取状态机当前所处的状态。
func (m *Machine) Current() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Trigger 触发一个事件，驱动状态机从当前状态转移至下一个目标状态。
// 关键逻辑：验证转移合法性 -> 执行 Handler -> 更新状态 -> 输出审计日志。
func (m *Machine) Trigger(ctx context.Context, event Event, args ...any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	from := m.current
	to, ok := m.transitions[from][event]
	if !ok {
		return fmt.Errorf("invalid event %s for state %s", event, from)
	}

	// 如果注册了转移处理器，则执行特定的业务逻辑。
	if handler, ok := m.handlers[from][to]; ok {
		if err := handler(ctx, from, to, args...); err != nil {
			return fmt.Errorf("handler failed for transition %s -> %s: %w", from, to, err)
		}
	}

	m.current = to

	// 输出结构化审计日志，用于追踪业务状态变化。
	slog.InfoContext(ctx, "fsm state transitioned",
		"from", string(from),
		"to", string(to),
		"event", string(event),
	)

	return nil
}
