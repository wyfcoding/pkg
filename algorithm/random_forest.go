package algorithm

import (
	"crypto/rand"
	"encoding/binary"
	"runtime"
	"sync"
)

const (
	defaultTreeMaxDepth  = 10
	defaultTreeMinSample = 5
)

// RandomForest 结构体实现了随机森林算法.
type RandomForest struct {
	trees    []*DecisionTree
	mu       sync.RWMutex
	numTrees int
}

// NewRandomForest 创建并返回一个新的 RandomForest 实例.
func NewRandomForest(numTrees int) *RandomForest {
	return &RandomForest{
		trees:    make([]*DecisionTree, numTrees),
		numTrees: numTrees,
		mu:       sync.RWMutex{},
	}
}

// Fit 训练随机森林模型.
func (rf *RandomForest) Fit(points []*DTPoint, labels []int) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	numWorkers := runtime.GOMAXPROCS(0)
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup

	for i := range rf.numTrees {
		wg.Add(1)

		go func(idxTree int) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			bootstrapPoints := make([]*DTPoint, len(points))
			bootstrapLabels := make([]int, len(labels))

			for j := range points {
				var b [4]byte
				_, _ = rand.Read(b[:])
				idx := int(binary.LittleEndian.Uint32(b[:]) % uint32(len(points)))
				bootstrapPoints[j] = points[idx]
				bootstrapLabels[j] = labels[idx]
			}

			tree := NewDecisionTree(defaultTreeMaxDepth, defaultTreeMinSample, CriterionGini)
			tree.Fit(bootstrapPoints, bootstrapLabels)
			rf.trees[idxTree] = tree
		}(i)
	}

	wg.Wait()
}

// Predict 使用训练好的随机森林模型对新的数据点进行预测.
func (rf *RandomForest) Predict(data []float64) int {
	rf.mu.RLock()
	defer rf.mu.RUnlock()

	votes := make(map[int]int)
	for _, tree := range rf.trees {
		label := tree.Predict(data)
		votes[label]++
	}

	maxVotes := 0
	maxLabel := 0

	for label, count := range votes {
		if count > maxVotes {
			maxVotes = count
			maxLabel = label
		}
	}

	return maxLabel
}
