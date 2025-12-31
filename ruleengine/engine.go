package ruleengine

import (
	"context"
	"fmt"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Result 规则执行结果
type Result struct {
	RuleID   string         `json:"rule_id"`
	Passed   bool           `json:"passed"`   // 表达式是否判定为 true
	Metadata map[string]any `json:"metadata"` // 命中后关联的元数据
}

// Rule 规则定义
type Rule struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Expression string         `json:"expression"` // DSL 表达式
	Metadata   map[string]any `json:"metadata"`   // 附加属性
	Priority   int            `json:"priority"`   // 优先级
}

// Engine 核心引擎
type Engine struct {
	mu       sync.RWMutex
	rules    map[string]*Rule
	programs map[string]*vm.Program
}

func NewEngine() *Engine {
	return &Engine{
		rules:    make(map[string]*Rule),
		programs: make(map[string]*vm.Program),
	}
}

// AddRule 添加或更新规则
func (e *Engine) AddRule(r Rule) error {
	// 架构优化：对于动态数据，编译时不指定特定的 Env，
	// 这样可以支持 map[string]any 类型的运行时数据。
	program, err := expr.Compile(r.Expression)
	if err != nil {
		return fmt.Errorf("failed to compile rule [%s]: %w", r.ID, err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules[r.ID] = &r
	e.programs[r.ID] = program
	return nil
}

// Execute 针对单条规则执行
func (e *Engine) Execute(ctx context.Context, ruleID string, facts map[string]any) (*Result, error) {
	e.mu.RLock()
	program, ok := e.programs[ruleID]
	rule, ruleOk := e.rules[ruleID]
	e.mu.RUnlock()

	if !ok || !ruleOk {
		return nil, fmt.Errorf("rule [%s] not found", ruleID)
	}

	output, err := expr.Run(program, facts)
	if err != nil {
		return nil, fmt.Errorf("execution error on rule [%s]: %w", ruleID, err)
	}

	passed, ok := output.(bool)
	if !ok {
		// 容错处理：如果表达式结果不是 bool，但执行成功，通常视为未命中
		return &Result{RuleID: ruleID, Passed: false, Metadata: rule.Metadata}, nil
	}

	return &Result{
		RuleID:   ruleID,
		Passed:   passed,
		Metadata: rule.Metadata,
	}, nil
}

// ExecuteAll 执行所有规则
func (e *Engine) ExecuteAll(ctx context.Context, facts map[string]any) ([]*Result, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var results []*Result
	for id, program := range e.programs {
		output, err := expr.Run(program, facts)
		if err != nil {
			continue
		}

		if passed, ok := output.(bool); ok && passed {
			results = append(results, &Result{
				RuleID:   id,
				Passed:   true,
				Metadata: e.rules[id].Metadata,
			})
		}
	}
	return results, nil
}
