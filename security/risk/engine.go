package risk

import (
	"context"
	"log/slog"

	"github.com/wyfcoding/pkg/ruleengine"
)

// DynamicRiskEngine 是基于可编程规则引擎实现的高级风控器。
// 支持在不重启服务的情况下，通过修改规则表达式动态调整风控策略。
type DynamicRiskEngine struct {
	re     *ruleengine.Engine // 底层规则执行引.
	logger *slog.Logger       // 日志记录.
}

// NewDynamicRiskEngine 初始化并返回一个新的动态风控引擎实例。
func NewDynamicRiskEngine(logger *slog.Logger) (*DynamicRiskEngine, error) {
	engine := ruleengine.NewEngine(logger)
	dre := &DynamicRiskEngine{
		re:     engine,
		logger: logger,
	}
	if err := dre.initDefaultRules(); err != nil {
		return nil, err
	}
	return dre, nil
}

// initDefaultRules 预置常用的风控规则模版。
func (e *DynamicRiskEngine) initDefaultRules() error {
	// 规则 1: 特定黑名单用户或 IP 拦.
	if err := e.re.AddRule(ruleengine.Rule{
		ID:         "R001",
		Name:       "Blacklist Check",
		Expression: "user_id == 888 || client_ip == '1.2.3.4'",
		Metadata:   map[string]any{"level": Reject, "code": "USER_IN_BLACKLIST"},
	}); err != nil {
		return err
	}

	// 规则 2: 大额交易安全性检查（未实名拦截.
	if err := e.re.AddRule(ruleengine.Rule{
		ID:         "R002",
		Name:       "High Value Unverified",
		Expression: "amount > 100000 && !is_real_name",
		Metadata:   map[string]any{"level": Reject, "code": "HIGH_VALUE_NO_VERIFY"},
	}); err != nil {
		return err
	}

	// 规则 3: 异常高频行为审.
	if err := e.re.AddRule(ruleengine.Rule{
		ID:         "R003",
		Name:       "Suspicious Frequency",
		Expression: "daily_count > 50",
		Metadata:   map[string]any{"level": Review, "code": "HIGH_FREQUENCY"},
	}); err != nil {
		return err
	}
	return nil
}

// Assess 执行规则评估。
// 判定策略：若命中多个规则，严格遵循优先级 Reject > Review > Pass。
func (e *DynamicRiskEngine) Assess(ctx context.Context, _ string, data map[string]any) (*Assessment, error) {
	hits, err := e.re.ExecuteAll(ctx, data)
	if err != nil {
		return nil, err
	}

	finalAssessment := &Assessment{Level: Pass, Code: "OK", Score: 0}

	for _, hit := range hits {
		levelStr, ok := hit.Metadata["level"].(Level)
		if !ok {
			continue
		}
		// 一票否决逻辑：一旦命中 Reject，立即返.
		if levelStr == Reject {
			code, _ := hit.Metadata["code"].(string)
			return &Assessment{
				Level:  Reject,
				Code:   code,
				Reason: "rule hit: " + hit.RuleID,
			}, nil
		}
		// 升级判定逻辑：如果当前是 Pass，可以升级为 Revie.
		if levelStr == Review {
			finalAssessment.Level = Review
			finalAssessment.Code, _ = hit.Metadata["code"].(string)
		}
	}

	return finalAssessment, nil
}
