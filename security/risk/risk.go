package risk

import (
	"context"
)

// Level 定义风险等级
type Level string

const (
	Pass   Level = "PASS"   // 放行
	Review Level = "REVIEW" // 需人工审核或二次验证
	Reject Level = "REJECT" // 拦截拒绝
)

// Assessment 风控评估结果
type Assessment struct {
	Level  Level  `json:"level"`
	Code   string `json:"code"`   // 风险代码，如 "RISK_HIGH_VALUE"
	Reason string `json:"reason"` // 风险原因描述
	Score  int    `json:"score"`  // 风险评分 (0-100)
}

// Evaluator 风险评估器接口
type Evaluator interface {
	// Assess 执行实时风控评估
	// action: 业务动作名称 (如 "order.create")
	// data: 评估所需的上下文数据 (用户ID, 金额, 设备指纹等)
	Assess(ctx context.Context, action string, data map[string]any) (*Assessment, error)
}

// BaseEvaluator 提供默认的简单风控实现 (Fail-Open 策略)
type BaseEvaluator struct{}

func (e *BaseEvaluator) Assess(ctx context.Context, action string, data map[string]any) (*Assessment, error) {
	// 默认实现为放行，具体业务需包装或替换此实现
	return &Assessment{Level: Pass, Code: "DEFAULT_PASS", Score: 0}, nil
}
