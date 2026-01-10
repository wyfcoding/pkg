package ml

import (
	"math"
	"sync"

	"github.com/wyfcoding/pkg/xerrors"
)

const (
	minLogProb = -1e10
	tinyProb   = 1e-300
	maxProbVal = -1e20
)

// HiddenMarkovModel 结构体实现了隐马尔可夫模型 (HMM).
type HiddenMarkovModel struct {
	stateIdx       map[string]int
	obsIdx         map[string]int
	states         []string
	observations   []string
	transitionProb []float64
	emissionProb   []float64
	initialProb    []float64
	mu             sync.RWMutex
}

// NewHiddenMarkovModel 创建并返回一个新的 HiddenMarkovModel 实例.
func NewHiddenMarkovModel(states, observations []string) *HiddenMarkovModel {
	numStates := len(states)
	numObs := len(observations)

	hmm := &HiddenMarkovModel{
		states:         states,
		observations:   observations,
		stateIdx:       make(map[string]int, numStates),
		obsIdx:         make(map[string]int, numObs),
		transitionProb: make([]float64, numStates*numStates),
		emissionProb:   make([]float64, numStates*numObs),
		initialProb:    make([]float64, numStates),
		mu:             sync.RWMutex{},
	}

	for i, s := range states {
		hmm.stateIdx[s] = i
	}

	for i, o := range observations {
		hmm.obsIdx[o] = i
	}

	return hmm
}

// SetTransitionProb 设置状态转移概率.
func (hmm *HiddenMarkovModel) SetTransitionProb(from, to string, prob float64) {
	hmm.mu.Lock()
	defer hmm.mu.Unlock()

	i, ok1 := hmm.stateIdx[from]
	j, ok2 := hmm.stateIdx[to]
	if ok1 && ok2 {
		numStates := len(hmm.states)
		hmm.transitionProb[i*numStates+j] = prob
	}
}

// SetEmissionProb 设置观测概率.
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

// SetInitialProb 设置初始状态概率.
func (hmm *HiddenMarkovModel) SetInitialProb(state string, prob float64) {
	hmm.mu.Lock()
	defer hmm.mu.Unlock()

	if i, ok := hmm.stateIdx[state]; ok {
		hmm.initialProb[i] = prob
	}
}

func (hmm *HiddenMarkovModel) safeLog(p float64) float64 {
	if p <= tinyProb {
		return minLogProb
	}

	return math.Log(p)
}

// Viterbi 算法用于寻找给定一个观测序列时，最有可能的隐藏状态序列.
func (hmm *HiddenMarkovModel) Viterbi(observations []string) []string {
	if len(observations) == 0 {
		return nil
	}

	hmm.mu.RLock()
	defer hmm.mu.RUnlock()

	numObsSeq := len(observations)
	numStates := len(hmm.states)
	numObsLib := len(hmm.observations)

	obsIndices := make([]int, numObsSeq)
	for t, obs := range observations {
		if idx, ok := hmm.obsIdx[obs]; ok {
			obsIndices[t] = idx
		} else {
			obsIndices[t] = -1
		}
	}

	dp := make([]float64, numObsSeq*numStates)
	path := make([]int, numObsSeq*numStates)

	hmm.viterbiInit(dp, obsIndices[0], numStates, numObsLib)
	hmm.viterbiIter(dp, path, obsIndices, numStates, numObsLib)

	return hmm.viterbiBacktrack(dp, path, numObsSeq, numStates)
}

func (hmm *HiddenMarkovModel) viterbiInit(dp []float64, obs0, numStates, numObsLib int) {
	for j := range numStates {
		emitP := 0.0
		if obs0 != -1 {
			emitP = hmm.emissionProb[j*numObsLib+obs0]
		}

		dp[j] = hmm.safeLog(hmm.initialProb[j]) + hmm.safeLog(emitP)
	}
}

func (hmm *HiddenMarkovModel) viterbiIter(dp []float64, path, obsIdx []int, numStates, numObsLib int) {
	for t := 1; t < len(obsIdx); t++ {
		obsT := obsIdx[t]
		baseCurr := t * numStates
		basePrev := (t - 1) * numStates

		for j := range numStates {
			maxP := maxProbVal
			maxPrev := 0

			emitP := 0.0
			if obsT != -1 {
				emitP = hmm.emissionProb[j*numObsLib+obsT]
			}

			logEmit := hmm.safeLog(emitP)

			for k := range numStates {
				transP := hmm.transitionProb[k*numStates+j]
				prob := dp[basePrev+k] + hmm.safeLog(transP)
				if prob > maxP {
					maxP = prob
					maxPrev = k
				}
			}

			dp[baseCurr+j] = maxP + logEmit
			path[baseCurr+j] = maxPrev
		}
	}
}

func (hmm *HiddenMarkovModel) viterbiBacktrack(dp []float64, path []int, numObsSeq, numStates int) []string {
	result := make([]string, numObsSeq)
	maxP := maxProbVal
	maxIdx := 0

	baseLast := (numObsSeq - 1) * numStates
	for j := range numStates {
		if dp[baseLast+j] > maxP {
			maxP = dp[baseLast+j]
			maxIdx = j
		}
	}

	result[numObsSeq-1] = hmm.states[maxIdx]
	for t := numObsSeq - 1; t > 0; t-- {
		maxIdx = path[t*numStates+maxIdx]
		result[t-1] = hmm.states[maxIdx]
	}

	return result
}

// Validate 校验模型参数是否完整合法.
func (hmm *HiddenMarkovModel) Validate() error {
	hmm.mu.RLock()
	defer hmm.mu.RUnlock()

	numStates := len(hmm.states)
	numObsLib := len(hmm.observations)

	var sumInit float64
	for _, p := range hmm.initialProb {
		sumInit += p
	}

	if sumInit < 0.99 || sumInit > 1.01 {
		return xerrors.ErrInvalidProbabilities
	}

	for i := range numStates {
		var sumTrans float64
		for j := range numStates {
			sumTrans += hmm.transitionProb[i*numStates+j]
		}

		if sumTrans < 0.99 || sumTrans > 1.01 {
			return xerrors.ErrInvalidProbabilities
		}
	}

	for i := range numStates {
		var sumEmit float64
		for k := range numObsLib {
			sumEmit += hmm.emissionProb[i*numObsLib+k]
		}

		if sumEmit < 0.99 || sumEmit > 1.01 {
			return xerrors.ErrInvalidProbabilities
		}
	}

	return nil
}
