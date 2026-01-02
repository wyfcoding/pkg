package algorithm

import (
	"fmt"
	"math"
	"sync"
)

// HiddenMarkovModel 结构体实现了隐马尔可夫模型 (HMM)。
// HMM 是一种统计模型，用于描述一个含有隐含未知参数的马尔可夫过程。
// 它的难点在于这些未知参数的确定，以及如何利用这些参数去解码隐藏的状态。
type HiddenMarkovModel struct {
	states         []string                      // 可能的隐藏状态集合。
	observations   []string                      // 可能的观测符号集合。
	transitionProb map[string]map[string]float64 // 状态转移概率：从一个隐藏状态转移到另一个隐藏状态的概率 P(S_t+1 | S_t)。
	emissionProb   map[string]map[string]float64 // 观测概率（或发射概率）：在给定隐藏状态下观测到某个符号的概率 P(O_t | S_t)。
	initialProb    map[string]float64            // 初始状态概率：在时间 t=0 时处于某个隐藏状态的概率 P(S_0)。
	mu             sync.RWMutex                  // 读写锁，用于保护模型参数的并发访问。
}

// NewHiddenMarkovModel 创建并返回一个新的 HiddenMarkovModel 实例。
// states: 模型的隐藏状态集合。
// observations: 模型的观测符号集合。
func NewHiddenMarkovModel(states, observations []string) *HiddenMarkovModel {
	return &HiddenMarkovModel{
		states:         states,
		observations:   observations,
		transitionProb: make(map[string]map[string]float64),
		emissionProb:   make(map[string]map[string]float64),
		initialProb:    make(map[string]float64),
	}
}

// SetTransitionProb 设置状态转移概率。
// from: 起始隐藏状态。
// to: 目标隐藏状态。
// prob: 从 from 转移到 to 的概率。
func (hmm *HiddenMarkovModel) SetTransitionProb(from, to string, prob float64) {
	hmm.mu.Lock()         // 加写锁以确保线程安全。
	defer hmm.mu.Unlock() // 确保函数退出时解锁。

	if hmm.transitionProb[from] == nil {
		hmm.transitionProb[from] = make(map[string]float64)
	}
	hmm.transitionProb[from][to] = prob
}

// SetEmissionProb 设置观测概率。
// state: 隐藏状态。
// obs: 观测符号。
// prob: 在给定 state 下观测到 obs 的概率。
func (hmm *HiddenMarkovModel) SetEmissionProb(state, obs string, prob float64) {
	hmm.mu.Lock()
	defer hmm.mu.Unlock()

	if hmm.emissionProb[state] == nil {
		hmm.emissionProb[state] = make(map[string]float64)
	}
	hmm.emissionProb[state][obs] = prob
}

// SetInitialProb 设置初始状态概率。
// state: 隐藏状态。
// prob: 在时间 t=0 时处于 state 的概率。
func (hmm *HiddenMarkovModel) SetInitialProb(state string, prob float64) {
	hmm.mu.Lock()
	defer hmm.mu.Unlock()

	hmm.initialProb[state] = prob
}

// Viterbi 算法用于寻找给定一个观测序列时，最有可能的隐藏状态序列。
func (hmm *HiddenMarkovModel) Viterbi(observations []string) []string {
	if len(observations) == 0 {
		return nil
	}

	hmm.mu.RLock()
	defer hmm.mu.RUnlock()

	n := len(observations)
	m := len(hmm.states)

	// 辅助函数：安全对数计算，防止 log(0)
	safeLog := func(p float64) float64 {
		if p <= 0 {
			return -1e10 // 使用一个极小的负数代替负无穷，保持数值稳定
		}
		return math.Log(p)
	}

	dp := make([][]float64, n)
	path := make([][]int, n)
	for i := range n {
		dp[i] = make([]float64, m)
		path[i] = make([]int, m)
	}

	// 1. 初始化 (t=0)
	for j, state := range hmm.states {
		dp[0][j] = safeLog(hmm.initialProb[state]) + safeLog(hmm.emissionProb[state][observations[0]])
	}

	// 2. 递推 (t=1..n-1)
	for i := 1; i < n; i++ {
		for j, state := range hmm.states {
			maxProb := -1e20 // 初始极小值
			maxK := 0

			for k, prevState := range hmm.states {
				prob := dp[i-1][k] + safeLog(hmm.transitionProb[prevState][state])
				if prob > maxProb {
					maxProb = prob
					maxK = k
				}
			}

			dp[i][j] = maxProb + safeLog(hmm.emissionProb[state][observations[i]])
			path[i][j] = maxK
		}
	}

	// 3. 回溯
	result := make([]string, n)
	maxProb := -1e20
	maxIdx := 0

	for j := range m {
		if dp[n-1][j] > maxProb {
			maxProb = dp[n-1][j]
			maxIdx = j
		}
	}

	result[n-1] = hmm.states[maxIdx]
	for i := n - 1; i > 0; i-- {
		maxIdx = path[i][maxIdx]
		result[i-1] = hmm.states[maxIdx]
	}

	return result
}

// Validate 校验模型参数是否完整合法 (各概率分布之和是否近似为 1)
func (hmm *HiddenMarkovModel) Validate() error {
	hmm.mu.RLock()
	defer hmm.mu.RUnlock()

	const epsilon = 1e-6

	// 1. 校验初始概率
	sumInit := 0.0
	for _, p := range hmm.initialProb {
		sumInit += p
	}
	if math.Abs(sumInit-1.0) > epsilon {
		return fmt.Errorf("initial probabilities sum to %f, expected 1.0", sumInit)
	}

	// 2. 校验转移概率
	for _, state := range hmm.states {
		sumTrans := 0.0
		for _, nextState := range hmm.states {
			sumTrans += hmm.transitionProb[state][nextState]
		}
		if math.Abs(sumTrans-1.0) > epsilon {
			return fmt.Errorf("transition probabilities from state %s sum to %f, expected 1.0", state, sumTrans)
		}
	}

	return nil
}
