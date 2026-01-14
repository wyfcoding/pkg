// Package ruleengine 提供了基于 expr 库的动态规则评估引擎。
package ruleengine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

var (
	// ErrRuleNotFound 规则未找到。
	ErrRuleNotFound = errors.New("rule not found")
	// ErrInvalidRuleOutput 规则输出类型错误。
	ErrInvalidRuleOutput = errors.New("rule output type invalid")
)

// Result 定义了单条规则执行的结果。
type Result struct {
	Metadata map[string]any `json:"metadata"` // 规则关联的静态元数据。
	RuleID   string         `json:"rule_id"`  // 触发判定的规则唯一 ID。
	Passed   bool           `json:"passed"`   // 判定结果。
}

// Rule 定义了一个业务判定规则。
type Rule struct {
	Metadata   map[string]any `json:"metadata"`   // 业务载荷数据。
	ID         string         `json:"id"`         // 规则唯一标识。
	Name       string         `json:"name"`       // 规则友好名称。
	Expression string         `json:"expression"` // DSL 判定表达式。
	Priority   int            `json:"priority"`   // 执行优先级。
}

// Engine 负责管理和执行大规模规则集。
type Engine struct {
	logger   *slog.Logger
	rules    map[string]*Rule
	programs map[string]*vm.Program
	mu       sync.RWMutex
}

// NewEngine 初始化并返回一个新的规则引擎。
func NewEngine(logger *slog.Logger) *Engine {
	return &Engine{
		logger:   logger.With("module", "rule_engine"),
		rules:    make(map[string]*Rule),
		programs: make(map[string]*vm.Program),
	}
}

// AddRule 向引擎中动态添加并编译一条新规则。
func (e *Engine) AddRule(rule Rule) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	program, err := expr.Compile(rule.Expression)
	if err != nil {
		return fmt.Errorf("failed to compile rule [%s]: %w", rule.ID, err)
	}

	e.rules[rule.ID] = &rule
	e.programs[rule.ID] = program

	return nil
}

// Execute 针对给定的事实数据评估单条已注册的规则。
func (e *Engine) Execute(ctx context.Context, ruleID string, facts map[string]any) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	e.mu.RLock()
	program, exists := e.programs[ruleID]
	rule, ruleExists := e.rules[ruleID]
	e.mu.RUnlock()

	if !exists || !ruleExists {
		return Result{}, fmt.Errorf("%w: rule [%s]", ErrRuleNotFound, ruleID)
	}

	output, err := expr.Run(program, facts)
	if err != nil {
		return Result{}, fmt.Errorf("failed to execute rule [%s]: %w", ruleID, err)
	}

	passed, ok := output.(bool)
	if !ok {
		return Result{}, fmt.Errorf("%w: rule [%s] output is not boolean (got %T)", ErrInvalidRuleOutput, ruleID, output)
	}

	return Result{
		RuleID:   ruleID,
		Passed:   passed,
		Metadata: rule.Metadata,
	}, nil
}

// ExecuteAll 针对给定的事实数据（Facts）并行或顺序评估所有已注册规则。
func (e *Engine) ExecuteAll(ctx context.Context, facts map[string]any) ([]Result, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	results := make([]Result, 0, len(e.rules))

	for id, program := range e.programs {
		output, err := expr.Run(program, facts)
		if err != nil {
			e.logger.Warn("failed to execute rule", "rule_id", id, "error", err)

			continue
		}

		passed, ok := output.(bool)
		if !ok {
			e.logger.Warn("rule output is not boolean", "rule_id", id, "output_type", fmt.Sprintf("%T", output))

			continue
		}

		if passed {
			results = append(results, Result{
				RuleID:   id,
				Passed:   true,
				Metadata: e.rules[id].Metadata,
			})
		}
	}

	return results, nil
}
