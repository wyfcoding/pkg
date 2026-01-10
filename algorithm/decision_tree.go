// Package algorithm 提供了高性能算法实现。
package algorithm

import (
	"math"
	"slices"
	"sync"
)

// DTPoint 结构体代表数据集中的一个数据点。
type DTPoint struct {
	Data []float64
}

// TreeNode 结构体代表决策树的一个节点。
type TreeNode struct {
	Feature   int       // 分裂特征的索引。
	Threshold float64   // 分裂阈值。
	Left      *TreeNode // 左子节点。
	Right     *TreeNode // 右子节点。
	Label     int       // 叶子节点的预测标签。
	IsLeaf    bool      // 标记当前节点是否为叶子节点。
}

const (
	// CriterionGini 使用基尼系数作为分裂标准。
	CriterionGini = "gini"
	// CriterionEntropy 使用信息熵作为分裂标准。
	CriterionEntropy = "entropy"
)

// DecisionTree 结构体实现了决策树模型。
type DecisionTree struct {
	root      *TreeNode
	maxDepth  int
	minSample int
	criterion string
	mu        sync.RWMutex
}

// NewDecisionTree 创建并返回一个新的 DecisionTree 实例。
func NewDecisionTree(maxDepth, minSample int, criterion string) *DecisionTree {
	selectedCriterion := criterion
	if selectedCriterion == "" {
		selectedCriterion = CriterionGini
	}

	return &DecisionTree{
		root:      nil,
		maxDepth:  maxDepth,
		minSample: minSample,
		criterion: selectedCriterion,
	}
}

// Fit 训练决策树模型。
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
			IsLeaf: true,
			Label:  dt.getMajorityLabel(labelCount),
		}
	}

	bestFeature, bestThreshold := dt.findBestSplit(points, labels)
	if bestFeature == -1 {
		return &TreeNode{
			IsLeaf: true,
			Label:  dt.getMajorityLabel(labelCount),
		}
	}

	leftPoints, leftLabels, rightPoints, rightLabels := dt.splitData(points, labels, bestFeature, bestThreshold)

	return &TreeNode{
		Feature:   bestFeature,
		Threshold: bestThreshold,
		Left:      dt.buildTree(leftPoints, leftLabels, depth+1),
		Right:     dt.buildTree(rightPoints, rightLabels, depth+1),
		IsLeaf:    false,
	}
}

func (dt *DecisionTree) countLabels(labels []int) map[int]int {
	counts := make(map[int]int)
	for _, label := range labels {
		counts[label]++
	}

	return counts
}

func (dt *DecisionTree) isTerminal(numSamples, depth int, counts map[int]int) bool {
	if numSamples < dt.minSample || depth >= dt.maxDepth {
		return true
	}

	// 检查是否为纯净节点
	return len(counts) == 1
}

func (dt *DecisionTree) getMajorityLabel(counts map[int]int) int {
	maxCount := -1
	majorityLabel := 0

	for label, count := range counts {
		if count > maxCount {
			maxCount = count
			majorityLabel = label
		}
	}

	return majorityLabel
}

func (dt *DecisionTree) findBestSplit(points []*DTPoint, labels []int) (int, float64) {
	bestGain := -1.0
	bestFeature := -1
	bestThreshold := 0.0
	dim := len(points[0].Data)

	for featureIdx := range dim {
		threshold, gain := dt.findBestThresholdForFeature(points, labels, featureIdx)
		if gain > bestGain {
			bestGain = gain
			bestFeature = featureIdx
			bestThreshold = threshold
		}
	}

	return bestFeature, bestThreshold
}

func (dt *DecisionTree) findBestThresholdForFeature(points []*DTPoint, labels []int, featureIdx int) (float64, float64) {
	numSamples := len(points)
	values := make([]float64, numSamples)

	for i, point := range points {
		values[i] = point.Data[featureIdx]
	}

	slices.Sort(values)

	bestFeatureGain := -1.0
	bestFeatureThreshold := 0.0

	for i := 0; i < numSamples-1; i++ {
		if values[i] == values[i+1] {
			continue
		}

		threshold := (values[i] + values[i+1]) / 2.0
		gain := dt.evaluateSplit(points, labels, featureIdx, threshold)

		if gain > bestFeatureGain {
			bestFeatureGain = gain
			bestFeatureThreshold = threshold
		}
	}

	return bestFeatureThreshold, bestFeatureGain
}

func (dt *DecisionTree) evaluateSplit(points []*DTPoint, labels []int, featureIdx int, threshold float64) float64 {
	var leftLabels, rightLabels []int

	for i, point := range points {
		if point.Data[featureIdx] <= threshold {
			leftLabels = append(leftLabels, labels[i])
		} else {
			rightLabels = append(rightLabels, labels[i])
		}
	}

	if len(leftLabels) == 0 || len(rightLabels) == 0 {
		return -1.0
	}

	return dt.calculateGain(labels, leftLabels, rightLabels)
}

func (dt *DecisionTree) splitData(points []*DTPoint, labels []int, feature int, threshold float64) ([]*DTPoint, []int, []*DTPoint, []int) {
	var leftPoints, rightPoints []*DTPoint
	var leftLabels, rightLabels []int

	for i, point := range points {
		if point.Data[feature] <= threshold {
			leftPoints = append(leftPoints, point)
			leftLabels = append(leftLabels, labels[i])
		} else {
			rightPoints = append(rightPoints, point)
			rightLabels = append(rightLabels, labels[i])
		}
	}

	return leftPoints, leftLabels, rightPoints, rightLabels
}

func (dt *DecisionTree) calculateGain(parent, left, right []int) float64 {
	if dt.criterion == CriterionEntropy {
		return dt.calculateInformationGain(parent, left, right)
	}

	return dt.calculateGiniGain(parent, left, right)
}

func (dt *DecisionTree) calculateInformationGain(parent, left, right []int) float64 {
	parentEntropy := dt.entropy(parent)
	leftWeight := float64(len(left)) / float64(len(parent))
	rightWeight := float64(len(right)) / float64(len(parent))

	return parentEntropy - (leftWeight*dt.entropy(left) + rightWeight*dt.entropy(right))
}

func (dt *DecisionTree) calculateGiniGain(parent, left, right []int) float64 {
	parentGini := dt.gini(parent)
	leftWeight := float64(len(left)) / float64(len(parent))
	rightWeight := float64(len(right)) / float64(len(parent))

	return parentGini - (leftWeight*dt.gini(left) + rightWeight*dt.gini(right))
}

func (dt *DecisionTree) gini(labels []int) float64 {
	numLabels := len(labels)
	if numLabels == 0 {
		return 0
	}

	counts := dt.countLabels(labels)
	impurity := 1.0

	for _, count := range counts {
		probability := float64(count) / float64(numLabels)
		impurity -= probability * probability
	}

	return impurity
}

func (dt *DecisionTree) entropy(labels []int) float64 {
	numLabels := len(labels)
	if numLabels == 0 {
		return 0
	}

	counts := dt.countLabels(labels)
	var result float64

	for _, count := range counts {
		probability := float64(count) / float64(numLabels)
		if probability > 0 {
			result -= probability * math.Log2(probability)
		}
	}

	return result
}

// Predict 使用训练好的决策树对新的数据点进行预测。
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