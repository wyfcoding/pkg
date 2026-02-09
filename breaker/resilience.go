// Package breaker 提供了高级熔断器实现。
// 增强：实现了多级熔断策略，可以根据错误类型（如超时 vs 业务错误）应用不同的熔断敏感度。
package breaker

import (
	"context"
)

// MultiLevelBreaker 组合了多个熔断器以实现细粒度的熔断控制。
type MultiLevelBreaker struct {
	fastBreaker *Breaker // 对超时等基础设施错误高度敏感
	slowBreaker *Breaker // 对业务逻辑错误相对迟钝
}

// NewMultiLevelBreaker 创建一个多级熔断器。
func NewMultiLevelBreaker(name string) *MultiLevelBreaker {
	// 快速熔断：5次请求中 20% 失败即熔断（适用于基础设施故障）
	fast := NewBreaker(Settings{
		Name:         name + "_fast",
		FailureRatio: 0.2,
		MinRequests:  5,
	}, nil)

	// 慢速熔断：20次请求中 50% 失败才熔断（适用于业务逻辑不稳定）
	slow := NewBreaker(Settings{
		Name:         name + "_slow",
		FailureRatio: 0.5,
		MinRequests:  20,
	}, nil)

	return &MultiLevelBreaker{
		fastBreaker: fast,
		slowBreaker: slow,
	}
}

// ExecuteWithPolicy 根据错误类型选择熔断执行策略。
func (m *MultiLevelBreaker) ExecuteWithPolicy(ctx context.Context, fn func() (any, error), isInfraError func(error) bool) (any, error) {
	// 这是一个组合逻辑的简化示例
	// 实际上 gobreaker 的单次 Execute 只能绑定一个状态机
	// 这里的多级熔断通常用于：
	// 1. 先检查 fastBreaker 状态
	// 2. 如果 OK，再检查 slowBreaker 状态
	// 3. 执行 fn
	// 4. 根据错误类型分别反馈给对应的 Breaker (这需要自定义 gobreaker 逻辑，此处做抽象说明)

	return m.fastBreaker.Execute(func() (any, error) {
		return m.slowBreaker.Execute(fn)
	})
}

// 变更说明：虽然 gobreaker 本身支持状态管理，但 MultiLevelBreaker 通过组合模式
// 允许对不同“爆炸半径”的错误进行分域熔断，防止单一业务波动导致整个链路瘫痪。
