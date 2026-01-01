package risk

import (
	"context"
	"log/slog"

	"github.com/wyfcoding/pkg/ruleengine"
)

// DynamicRiskEngine 基于规则引擎的高级风控实现
type DynamicRiskEngine struct {
	re     *ruleengine.Engine
	logger *slog.Logger
}

func NewDynamicRiskEngine(logger *slog.Logger) *DynamicRiskEngine {
	engine := ruleengine.NewEngine(logger)
	dre := &DynamicRiskEngine{
		re:     engine,
		logger: logger,
	}
	dre.initDefaultRules() // 初始化默认规则，生产环境可改为从数据库/配置中心加载
	return dre
}

func (e *DynamicRiskEngine) initDefaultRules() {
	// 规则 1: 黑名单拦截
	if err := e.re.AddRule(ruleengine.Rule{
		ID:         "R001",
		Name:       "Blacklist Check",
		Expression: "user_id == 888 || client_ip == '1.2.3.4'",
		Metadata:   map[string]any{"level": Reject, "code": "USER_IN_BLACKLIST"},
	}); err != nil {
		e.logger.Error("failed to add default rule R001", "error", err)
	}

	// 规则 2: 大额且未实名拦截
	if err := e.re.AddRule(ruleengine.Rule{
		ID:         "R002",
		Name:       "High Value Unverified",
		Expression: "amount > 100000 && !is_real_name",
		Metadata:   map[string]any{"level": Reject, "code": "HIGH_VALUE_NO_VERIFY"},
	}); err != nil {
		e.logger.Error("failed to add default rule R002", "error", err)
	}

	// 规则 3: 疑似欺诈行为审核
	if err := e.re.AddRule(ruleengine.Rule{
		ID:         "R003",
		Name:       "Suspicious Frequency",
		Expression: "daily_count > 50",
		Metadata:   map[string]any{"level": Review, "code": "HIGH_FREQUENCY"},
	}); err != nil {
		e.logger.Error("failed to add default rule R003", "error", err)
	}
}

func (e *DynamicRiskEngine) Assess(ctx context.Context, action string, data map[string]any) (*Assessment, error) {
	hits, err := e.re.ExecuteAll(ctx, data)
	if err != nil {
		return nil, err
	}

	// 策略选择：如果有多个规则命中，取风险最高的一个 (Reject > Review > Pass)
	finalAssessment := &Assessment{Level: Pass, Code: "OK", Score: 0}

	for _, hit := range hits {
		levelStr, ok := hit.Metadata["level"].(Level)
		if !ok {
			continue
		}
		if levelStr == Reject {
			code, _ := hit.Metadata["code"].(string)
			return &Assessment{
				Level:  Reject,
				Code:   code,
				Reason: "Rule hit: " + hit.RuleID,
			}, nil
		}
		if levelStr == Review {
			finalAssessment.Level = Review
			finalAssessment.Code, _ = hit.Metadata["code"].(string)
		}
	}

	return finalAssessment, nil
}
