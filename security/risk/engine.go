package risk

import (
	"context"
	"log/slog"
)

// SimpleRuleEngine 基于简单规则的评估器示例
type SimpleRuleEngine struct {
	logger *slog.Logger
}

func NewSimpleRuleEngine(logger *slog.Logger) *SimpleRuleEngine {
	return &SimpleRuleEngine{logger: logger}
}

func (e *SimpleRuleEngine) Assess(ctx context.Context, action string, data map[string]any) (*Assessment, error) {
	// 架构师提示：实际生产中此处应调用：
	// 1. 规则引擎 (如 Expr, Go-Rules)
	// 2. 外部风控微服务
	// 3. AI 模型推理接口

	amount, ok := data["amount"].(int64)
	if !ok {
		return &Assessment{Level: Pass}, nil
	}

	// 演示规则：单笔订单金额超过 100,000 (1000元) 且没有实名认证的进入审核
	if amount > 100000 {
		isRealName, _ := data["is_real_name"].(bool)
		if !isRealName {
			return &Assessment{
				Level:  Review,
				Code:   "HIGH_VALUE_NO_VERIFY",
				Reason: "大额交易且未实名认证",
				Score:  70,
			}, nil
		}
	}

	// 演示规则：黑名单用户拦截
	userID, _ := data["user_id"].(uint64)
	if userID == 888 { // 模拟黑名单 ID
		return &Assessment{
			Level:  Reject,
			Code:   "USER_IN_BLACKLIST",
			Reason: "用户处于风控黑名单",
			Score:  99,
		}, nil
	}

	return &Assessment{Level: Pass, Code: "OK", Score: 10}, nil
}
