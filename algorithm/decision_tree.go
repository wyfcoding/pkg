// Package algorithm 提供了高性能算法实现.
package algorithm

import (
	"math"
	"slices"
	"sync"
)

// DTPoint 结构体代表数据集中的一个数据点.
type DTPoint struct {
	Data []float64
}

// TreeNode 结构体代表决策树的一个节点.
type TreeNode struct {
	Left      *TreeNode
	Right     *TreeNode
	Threshold float64
	Feature   int
	Label     int
	IsLeaf    bool
}

const (
	// CriterionGini 使用基尼系数作为分裂标准.
	CriterionGini = "gini"
	// CriterionEntropy 使用信息熵作为分裂标准.
	CriterionEntropy = "entropy"
)

const (
	defaultSplitThreshold = 2.0
)

// DecisionTree 结构体实现了决策树模型.
type DecisionTree struct {
	root      *TreeNode
	criterion string
	mu        sync.RWMutex
	maxDepth  int
	minSample int
}

// NewDecisionTree 创建并返回一个新的 DecisionTree 实例.
func NewDecisionTree(maxDepth, minSample int, criterion string) *DecisionTree {
	selected := criterion
	if selected == "" {
		selected = CriterionGini
	}

	return &DecisionTree{
		root:      nil,
		maxDepth:  maxDepth,
		minSample: minSample,
		criterion: selected,
		mu:        sync.RWMutex{},
	}
}

// Fit 训练决策树模型.
func (dt *DecisionTree) Fit(points []*DTPoint, labels []int) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	dt.root = dt.buildTree(points, labels, 0)
}

func (dt *DecisionTree) buildTree(points []*DTPoint, labels []int, depth int) *TreeNode {
	numSamples := len(points)
	if numSamples == 0 {
		return nil
	}

	labelCount := dt.countLabels(labels)
	if dt.isTerminal(numSamples, depth, labelCount) {
		return &TreeNode{
			IsLeaf:    true,
			Label:     dt.getMajorityLabel(labelCount),
			Left:      nil,
			Right:     nil,
			Threshold: 0,
			Feature:   0,
		}
	}

	feat, thresh := dt.findBestSplit(points, labels)
	if feat == -1 {
		return &TreeNode{
			IsLeaf:    true,
			Label:     dt.getMajorityLabel(labelCount),
			Left:      nil,
			Right:     nil,
			Threshold: 0,
			Feature:   0,
		}
	}

	leftP, leftL, rightP, rightL := dt.splitData(points, labels, feat, thresh)

	return &TreeNode{
		Feature:   feat,
		Threshold: thresh,
		Left:      dt.buildTree(leftP, leftL, depth+1),
		Right:     dt.buildTree(rightP, rightL, depth+1),
		IsLeaf:    false,
		Label:     0,
	}
}

func (dt *DecisionTree) countLabels(labels []int) map[int]int {
	counts := make(map[int]int)
	for _, l := range labels {
		counts[l]++
	}

	return counts
}

func (dt *DecisionTree) isTerminal(numSamples, depth int, counts map[int]int) bool {
	if numSamples < dt.minSample || depth >= dt.maxDepth {
		return true
	}

	return len(counts) == 1
}

func (dt *DecisionTree) getMajorityLabel(counts map[int]int) int {
	maxC := -1
	majority := 0

	for label, count := range counts {
		if count > maxC {
			maxC = count
			majority = label
		}
	}

	return majority
}

func (dt *DecisionTree) findBestSplit(points []*DTPoint, labels []int) (int, float64) {
	bestG := -1.0
	bestF := -1
	bestT := 0.0
	dim := len(points[0].Data)

	for fIdx := range dim {
		thresh, gain := dt.findBestThresholdForFeature(points, labels, fIdx)
		if gain > bestG {
			bestG = gain
			bestF = fIdx
			bestT = thresh
		}
	}

	return bestF, bestT
}

func (dt *DecisionTree) findBestThresholdForFeature(points []*DTPoint, labels []int, fIdx int) (float64, float64) {
	n := len(points)
	vals := make([]float64, n)

	for i, p := range points {
		vals[i] = p.Data[fIdx]
	}

	slices.Sort(vals)

	bestG := -1.0
	bestT := 0.0

	for i := range vals[:n-1] {
		if vals[i] == vals[i+1] {
			continue
		}

		thresh := (vals[i] + vals[i+1]) / defaultSplitThreshold
		gain := dt.evaluateSplit(points, labels, fIdx, thresh)

		if gain > bestG {
			bestG = gain
			bestT = thresh
		}
	}

	return bestT, bestG
}

func (dt *DecisionTree) evaluateSplit(points []*DTPoint, labels []int, fIdx int, thresh float64) float64 {
	var leftL, rightL []int

	for i, p := range points {
		if p.Data[fIdx] <= thresh {
			leftL = append(leftL, labels[i])
		} else {
			rightL = append(rightL, labels[i])
		}
	}

	if len(leftL) == 0 || len(rightL) == 0 {
		return -1.0
	}

	return dt.calculateGain(labels, leftL, rightL)
}

func (dt *DecisionTree) splitData(points []*DTPoint, labels []int, feat int, thresh float64) ([]*DTPoint, []int, []*DTPoint, []int) {
	var leftP, rightP []*DTPoint
	var leftL, rightL []int

	for i, p := range points {
		if p.Data[feat] <= thresh {
			leftP = append(leftP, p)
			leftL = append(leftL, labels[i])
		} else {
			rightP = append(rightP, p)
			rightL = append(rightL, labels[i])
		}
	}

	return leftP, leftL, rightP, rightL
}

func (dt *DecisionTree) calculateGain(parent, left, right []int) float64 {
	if dt.criterion == CriterionEntropy {
		return dt.calculateInformationGain(parent, left, right)
	}

	return dt.calculateGiniGain(parent, left, right)
}

func (dt *DecisionTree) calculateInformationGain(parent, left, right []int) float64 {
	pEntropy := dt.entropy(parent)
	lWeight := float64(len(left)) / float64(len(parent))
	rWeight := float64(len(right)) / float64(len(parent))

	return pEntropy - (lWeight*dt.entropy(left) + rWeight*dt.entropy(right))
}

func (dt *DecisionTree) calculateGiniGain(parent, left, right []int) float64 {
	pGini := dt.gini(parent)
	lWeight := float64(len(left)) / float64(len(parent))
	rWeight := float64(len(right)) / float64(len(parent))

	return pGini - (lWeight*dt.gini(left) + rWeight*dt.gini(right))
}

func (dt *DecisionTree) gini(labels []int) float64 {
	n := len(labels)
	if n == 0 {
		return 0
	}

	counts := dt.countLabels(labels)
	impurity := 1.0

	for _, count := range counts {
		prob := float64(count) / float64(n)
		impurity -= prob * prob
	}

	return impurity
}

func (dt *DecisionTree) entropy(labels []int) float64 {
	n := len(labels)
	if n == 0 {
		return 0
	}

	counts := dt.countLabels(labels)
	var res float64

	for _, count := range counts {
		prob := float64(count) / float64(n)
		if prob > 0 {
			res -= prob * math.Log2(prob)
		}
	}

	return res
}

// Predict 使用训练好的决策树对新的数据点进行预测.
func (dt *DecisionTree) Predict(data []float64) int {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	if dt.root == nil {
		return 0
	}

	return dt.predictNode(dt.root, data)
}

func (dt *DecisionTree) predictNode(node *TreeNode, data []float64) int {
	if node.IsLeaf {
		return node.Label
	}

	if data[node.Feature] <= node.Threshold {
		return dt.predictNode(node.Left, data)
	}

	return dt.predictNode(node.Right, data)
}
