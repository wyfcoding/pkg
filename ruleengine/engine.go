// Package ruleengine 提供了基于高性能 DSL 的动态规则判定引擎，支持实时风控与业务逻辑动态编排。
package ruleengine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// Result 定义了单条规则判定的结果。
type Result struct {
	RuleID   string         `json:"rule_id"`   // 触发判定的规则唯一 ID
	Passed   bool           `json:"passed"`    // 规则表达式计算结果是否为 true
	Metadata map[string]any `json:"metadata"`  // 规则关联的静态元数据 (如风险等级、拦截码)
}

// Rule 描述了一条可执行的业务规则。
type Rule struct {
	ID         string         `json:"id"`         // 规则唯一标识符
	Name       string         `json:"name"`       // 规则友好名称
	Expression string         `json:"expression"` // 符合 expr 语法的 DSL 判定表达式
	Metadata   map[string]any `json:"metadata"`   // 命中后需要传递给上层的业务数据
	Priority   int            `json:"priority"`   // 执行优先级（数值越大越高）
}

// Engine 封装了规则引擎的执行上下文与编译后的字节码缓存。
type Engine struct {
	mu       sync.RWMutex
	rules    map[string]*Rule       // 原始规则映射
	programs map[string]*vm.Program // 编译后的 VM 字节码，提升执行效率
	logger   *slog.Logger           // 结构化日志记录器
}

// NewEngine 初始化规则引擎。
func NewEngine(logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default().With("module", "rule_engine")
	}
	return &Engine{
		rules:    make(map[string]*Rule),
		programs: make(map[string]*vm.Program),
		logger:   logger,
	}
}

// AddRule 将规则表达式编译为字节码并存入引擎缓存。
// 流程：编译语法 -> 校验合法性 -> 写入内存。
func (e *Engine) AddRule(r Rule) error {
	// 预编译表达式，提升后续千万次执行的性能
	program, err := expr.Compile(r.Expression)
	if err != nil {
		return fmt.Errorf("failed to compile rule [%s]: %w", r.ID, err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.rules[r.ID] = &r
	e.programs[r.ID] = program

	e.logger.Info("rule added or updated successfully", "rule_id", r.ID, "name", r.Name)
	return nil
}

// Execute 针对给定的事实数据 (facts) 运行特定的规则。
func (e *Engine) Execute(ctx context.Context, ruleID string, facts map[string]any) (*Result, error) {
	e.mu.RLock()
	program, ok := e.programs[ruleID]
	rule, ruleOk := e.rules[ruleID]
	e.mu.RUnlock()

	if !ok || !ruleOk {
		return nil, fmt.Errorf("rule [%s] not found", ruleID)
	}

	// 在虚拟机中执行预编译的程序
	output, err := expr.Run(program, facts)
	if err != nil {
		return nil, fmt.Errorf("execution error on rule [%s]: %w", ruleID, err)
	}

	passed, ok := output.(bool)
	if !ok {
		// 容错：若结果非布尔值，则按未命中处理
		return &Result{RuleID: ruleID, Passed: false, Metadata: rule.Metadata}, nil
	}

	return &Result{
		RuleID:   ruleID,
		Passed:   passed,
		Metadata: rule.Metadata,
	}, nil
}

// ExecuteAll 遍历所有已注册规则进行全量判定，返回所有命中的结果集。
func (e *Engine) ExecuteAll(ctx context.Context, facts map[string]any) ([]*Result, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var results []*Result
	for id, program := range e.programs {
		output, err := expr.Run(program, facts)
		if err != nil {
			e.logger.ErrorContext(ctx, "rule execution failed", "rule_id", id, "error", err)
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
