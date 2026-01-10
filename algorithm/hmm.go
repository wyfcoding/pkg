package algorithm

import (
	"fmt"
	"math"
	"sync"
)

// HiddenMarkovModel 结构体实现了隐马尔可夫模型 (HMM)。
// 优化：内部使用数组代替 Map 存储概率，显著提升 Viterbi 算法性能 (避免哈希查找开销)。
type HiddenMarkovModel struct {
	states         []string // 状态列表
	observations   []string // 观测符号列表
	stateIdx       map[string]int
	obsIdx         map[string]int
	transitionProb []float64    // M x M 矩阵 (扁平化): trans[i][j] -> [i*M + j]
	emissionProb   []float64    // M x K 矩阵 (扁平化): emit[i][k] -> [i*K + k]
	initialProb    []float64    // M 向量
	mu             sync.RWMutex // 读写锁
}

// NewHiddenMarkovModel 创建并返回一个新的 HiddenMarkovModel 实例。
func NewHiddenMarkovModel(states, observations []string) *HiddenMarkovModel {
	m := len(states)
	k := len(observations)

	hmm := &HiddenMarkovModel{
		states:         states,
		observations:   observations,
		stateIdx:       make(map[string]int, m),
		obsIdx:         make(map[string]int, k),
		transitionProb: make([]float64, m*m),
		emissionProb:   make([]float64, m*k),
		initialProb:    make([]float64, m),
	}

	for i, s := range states {
		hmm.stateIdx[s] = i
	}
	for i, o := range observations {
		hmm.obsIdx[o] = i
	}

	return hmm
}

// SetTransitionProb 设置状态转移概率。
func (hmm *HiddenMarkovModel) SetTransitionProb(from, to string, prob float64) {
	hmm.mu.Lock()
	defer hmm.mu.Unlock()

	i, ok1 := hmm.stateIdx[from]
	j, ok2 := hmm.stateIdx[to]
	if ok1 && ok2 {
		m := len(hmm.states)
		hmm.transitionProb[i*m+j] = prob
	}
}

// SetEmissionProb 设置观测概率。
func (hmm *HiddenMarkovModel) SetEmissionProb(state, obs string, prob float64) {
	hmm.mu.Lock()
	defer hmm.mu.Unlock()

	i, ok1 := hmm.stateIdx[state]
	k, ok2 := hmm.obsIdx[obs]
	if ok1 && ok2 {
		numObs := len(hmm.observations)
		hmm.emissionProb[i*numObs+k] = prob
	}
}

// SetInitialProb 设置初始状态概率。
func (hmm *HiddenMarkovModel) SetInitialProb(state string, prob float64) {
	hmm.mu.Lock()
	defer hmm.mu.Unlock()

	if i, ok := hmm.stateIdx[state]; ok {
		hmm.initialProb[i] = prob
	}
}

// Viterbi 算法用于寻找给定一个观测序列时，最有可能的隐藏状态序列。
// 时间复杂度：O(T * M^2)，空间复杂度：O(T * M)
func (hmm *HiddenMarkovModel) Viterbi(observations []string) []string {
	if len(observations) == 0 {
		return nil
	}

	hmm.mu.RLock()
	defer hmm.mu.RUnlock()

	n := len(observations)
	m := len(hmm.states)
	numObs := len(hmm.observations)

	// 预先将观测序列转换为索引，避免循环中查找 Map
	obsIndices := make([]int, n)
	for t, obs := range observations {
		if idx, ok := hmm.obsIdx[obs]; ok {
			obsIndices[t] = idx
		} else {
			// 未知观测值，通常视其概率为0。使用 -1 标记。
			obsIndices[t] = -1
		}
	}

	// 辅助函数：安全对数计算
	const minLogProb = -1e10
	safeLog := func(p float64) float64 {
		if p <= 1e-300 { // 接近 0
			return minLogProb
		}
		return math.Log(p)
	}

	// dp[t][state_idx]
	dp := make([]float64, n*m)
	// path[t][state_idx] = prev_state_idx
	path := make([]int, n*m)

	// 1. 初始化 (t=0)
	obs0 := obsIndices[0]
	for j := 0; j < m; j++ {
		emitP := 0.0
		if obs0 != -1 {
			emitP = hmm.emissionProb[j*numObs+obs0]
		}
		dp[j] = safeLog(hmm.initialProb[j]) + safeLog(emitP)
	}

	// 2. 递推 (t=1..n-1)
	for t := 1; t < n; t++ {
		obsT := obsIndices[t]
		baseCurr := t * m
		basePrev := (t - 1) * m

		for j := 0; j < m; j++ { // 当前状态 j
			maxProb := -1e20
			maxPrevState := 0

			emitP := 0.0
			if obsT != -1 {
				emitP = hmm.emissionProb[j*numObs+obsT]
			}
			logEmit := safeLog(emitP)

			for k := 0; k < m; k++ { // 前一状态 k
				// prob = dp[t-1][k] * trans[k][j] * emit[j][obs]
				// log_prob = dp[t-1][k] + log(trans[k][j]) + log(emit)
				transP := hmm.transitionProb[k*m+j]
				prob := dp[basePrev+k] + safeLog(transP)
				if prob > maxProb {
					maxProb = prob
					maxPrevState = k
				}
			}

			dp[baseCurr+j] = maxProb + logEmit
			path[baseCurr+j] = maxPrevState
		}
	}

	// 3. 回溯
	result := make([]string, n)
	maxProb := -1e20
	maxIdx := 0

	baseLast := (n - 1) * m
	for j := 0; j < m; j++ {
		if dp[baseLast+j] > maxProb {
			maxProb = dp[baseLast+j]
			maxIdx = j
		}
	}

	result[n-1] = hmm.states[maxIdx]
	for t := n - 1; t > 0; t-- {
		maxIdx = path[t*m+maxIdx]
		result[t-1] = hmm.states[maxIdx]
	}

	return result
}

// Validate 校验模型参数是否完整合法 (各概率分布之和是否近似为 1)
func (hmm *HiddenMarkovModel) Validate() error {
	hmm.mu.RLock()
	defer hmm.mu.RUnlock()

	const epsilon = 1e-6
	m := len(hmm.states)
	numObs := len(hmm.observations)

	// 1. 校验初始概率
	sumInit := 0.0
	for _, p := range hmm.initialProb {
		sumInit += p
	}
	if math.Abs(sumInit-1.0) > epsilon {
		return fmt.Errorf("initial probabilities sum to %f, expected 1.0", sumInit)
	}

	// 2. 校验转移概率
	for i := 0; i < m; i++ {
		sumTrans := 0.0
		for j := 0; j < m; j++ {
			sumTrans += hmm.transitionProb[i*m+j]
		}
		if math.Abs(sumTrans-1.0) > epsilon {
			return fmt.Errorf("transition probabilities from state %s sum to %f, expected 1.0", hmm.states[i], sumTrans)
		}
	}

	// 3. 校验发射概率
	for i := 0; i < m; i++ {
		sumEmit := 0.0
		for k := 0; k < numObs; k++ {
			sumEmit += hmm.emissionProb[i*numObs+k]
		}
		if math.Abs(sumEmit-1.0) > epsilon {
			return fmt.Errorf("emission probabilities from state %s sum to %f, expected 1.0", hmm.states[i], sumEmit)
		}
	}

	return nil
}
