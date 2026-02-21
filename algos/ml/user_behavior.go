package ml

import "time"

// UserBehavior 用户行为分析实体
type UserBehavior struct {
	Timestamp time.Time
	IP        string
	UserAgent string
	Action    string
	UserID    uint64
}

// Analyze 分析
// Analyze 基于用户行为序列进行异常评分。
// 返回 0-1 之间的风险分数，分数越高表示越可疑。
func (ub *UserBehavior) Analyze(actions []string) float64 {
	if len(actions) == 0 {
		return 0.0
	}

	score := 0.0

	// 简单的规则判定示例
	// 1. 高频重复操作
	if detectRepetition(actions) {
		score += 0.4
	}

	// 2. 只有查看没有其他交互
	if detectPassiveOnly(actions) {
		score += 0.2
	}

	// 3. 敏感操作序列 (如: 登录 -> 改密 -> 提现)
	if detectSensitiveSequence(actions) {
		score += 0.3
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

func detectRepetition(actions []string) bool {
	if len(actions) < 5 {
		return false
	}
	last := actions[len(actions)-1]
	count := 0
	for i := len(actions) - 1; i >= 0; i-- {
		if actions[i] == last {
			count++
		} else {
			break
		}
	}
	return count >= 5
}

func detectPassiveOnly(actions []string) bool {
	for _, a := range actions {
		if a != "view" && a != "scroll" {
			return false
		}
	}
	return true
}

func detectSensitiveSequence(actions []string) bool {
	// 简单子序列匹配
	seq := []string{"login", "change_password", "withdraw"}
	idx := 0
	for _, a := range actions {
		if a == seq[idx] {
			idx++
			if idx == len(seq) {
				return true
			}
		}
	}
	return false
}
