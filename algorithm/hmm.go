package algorithm

import (
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
// 应用场景包括用户行为序列预测、订单异常检测、语音识别等。
// observations: 观测序列。
// 返回最可能的隐藏状态序列。
func (hmm *HiddenMarkovModel) Viterbi(observations []string) []string {
	hmm.mu.RLock()         // Viterbi算法只读取模型参数，因此加读锁。
	defer hmm.mu.RUnlock() // 确保函数退出时解锁。

	n := len(observations) // 观测序列的长度。
	m := len(hmm.states)   // 隐藏状态的数量。

	// dp 数组 (delta) 用于存储在时间 i 观测到 observations[i]，并且在状态 j 结束时的最大概率的对数。
	dp := make([][]float64, n)
	// path 数组 (psi) 用于存储在回溯时，达到当前状态 j 的前一个最可能状态的索引。
	path := make([][]int, n)

	for i := range n {
		dp[i] = make([]float64, m)
		path[i] = make([]int, m)
	}

	// 初始化 Viterbi 算法 (t=0 时刻)：
	// 对于第一个观测 observations[0]，计算到达每个隐藏状态的概率。
	for j, state := range hmm.states {
		// 使用对数概率避免浮点数下溢，并简化乘法为加法。
		dp[0][j] = math.Log(hmm.initialProb[state]) + math.Log(hmm.emissionProb[state][observations[0]])
	}

	// 递推步骤 (t=1 到 n-1 时刻)：
	// 对于每个时间步 i 和每个当前状态 j，计算达到该状态的最大对数概率。
	for i := 1; i < n; i++ {
		for j, state := range hmm.states { // 遍历当前时间步的所有可能状态。
			maxProb := math.Inf(-1) // 初始化为负无穷，因为是负的对数概率，最大概率对应最小的负值。
			maxK := 0               // 记录前一个时间步中导致最大概率的状态索引。

			for k, prevState := range hmm.states { // 遍历前一个时间步的所有可能状态。
				// 计算从前一个状态 k 转移到当前状态 j 的概率。
				prob := dp[i-1][k] + math.Log(hmm.transitionProb[prevState][state])
				if prob > maxProb {
					maxProb = prob
					maxK = k
				}
			}

			// 加上当前状态下观测到当前符号的发射概率。
			dp[i][j] = maxProb + math.Log(hmm.emissionProb[state][observations[i]])
			path[i][j] = maxK // 记录路径。
		}
	}

	// 回溯步骤 (t=n-1 时刻到 t=0 时刻)：
	// 找到最后一个观测的最可能状态，然后反向追踪路径以重建整个隐藏状态序列。
	result := make([]string, n) // 存储最终的隐藏状态序列。
	maxProb := math.Inf(-1)     // 最后一个时间步的最大对数概率。
	maxIdx := 0                 // 最后一个时间步中导致最大对数概率的状态索引。

	// 找到最后一个观测的最可能隐藏状态。
	for j := range m {
		if dp[n-1][j] > maxProb {
			maxProb = dp[n-1][j]
			maxIdx = j
		}
	}

	result[n-1] = hmm.states[maxIdx] // 确定最后一个状态。
	// 从后向前回溯路径，确定所有隐藏状态。
	for i := n - 1; i > 0; i-- {
		maxIdx = path[i][maxIdx]         // 根据 path 数组找到前一个最可能的状态索引。
		result[i-1] = hmm.states[maxIdx] // 确定该状态。
	}

	return result
}
