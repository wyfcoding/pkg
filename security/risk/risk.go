// Package risk 提供了业务风险控制的核心抽象，支持多等级风险判定与动态规则评估。
package risk

import (
	"context"
)

// Level 定义了风控判定的处理等级。
type Level string

const (
	Pass   Level = "PASS"   // 放行：无风险或风险极低，正常执行业务逻.
	Review Level = "REVIEW" // 审核：存在可疑风险，建议转人工审核或触发二次身份验证（如短信验证码.
	Reject Level = "REJECT" // 拒绝：明确的高风险行为，必须立即拦.
)

// Assessment 描述了单次风控评估的详细结论。
type Assessment struct { //nolint:govet // 风险评估结果结构，已优化对齐。
	Level  Level  `json:"level"`  // 最终判定等.
	Code   string `json:"code"`   // 命中的风险分类代码 (如: "RISK_GEO_IP_MISMATCH".
	Reason string `json:"reason"` // 风险命中的详细原因说.
	Score  int    `json:"score"`  // 风险量化评分 (0-100，分数越高风险越大.
}

// Evaluator 是风控评估器的标准接口。
type Evaluator interface {
	// Assess 根据业务动作和上下文数据执行实时风控评估。
	// 参数 action: 业务动作名称，用于匹配不同的评估策略 (如 "order.pay", "user.login")。
	// 参数 data: 包含用户 ID、IP、金额、设备指纹等评估所需的原始数据。
	Assess(ctx context.Context, action string, data map[string]any) (*Assessment, error)
}

// BaseEvaluator 提供默认的兜底风控实现，遵循 Fail-Open (故障放行) 策略。
// 用于在风控系统不可用或未配置规则时，保证业务连续性。
type BaseEvaluator struct{}

// Assess 执行基础的放行逻辑.
func (e *BaseEvaluator) Assess(_ context.Context, _ string, _ map[string]any) (*Assessment, error) {
	return &Assessment{Level: Pass, Code: "DEFAULT_PASS", Reason: "default_base_evaluator", Score: 0}, nil
}
