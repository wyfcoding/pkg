package algorithm

import (
	"math"
	"sort"
	"sync"
)

// Point 结构体代表数据集中的一个数据点。
// 它是决策树算法的输入特征。
// 注意：此结构体通常在外部定义，此处假设它包含一个名为 Data 的浮点数切片。
type DTPoint struct {
	Data []float64
}

// TreeNode 结构体代表决策树的一个节点。
// 它可以是内部节点（根据特征进行分裂），也可以是叶子节点（给出预测标签）。
type TreeNode struct {
	Feature   int       // 分裂特征的索引，如果是叶子节点则无意义。
	Threshold float64   // 分裂阈值，如果特征值小于等于此阈值则走向左子树，否则走向右子树。
	Left      *TreeNode // 左子节点，对应特征值小于等于阈值的情况。
	Right     *TreeNode // 右子节点，对应特征值大于阈值的情况。
	Label     int       // 叶子节点的预测标签。
	IsLeaf    bool      // 标记当前节点是否为叶子节点。
}

// DecisionTree 结构体实现了决策树模型。
type DecisionTree struct {
	root      *TreeNode
	maxDepth  int
	minSample int
	criterion string // "gini" 或 "entropy"
	mu        sync.RWMutex
}

// NewDecisionTree 创建并返回一个新的 DecisionTree 实例。
func NewDecisionTree(maxDepth, minSample int, criterion string) *DecisionTree {
	if criterion == "" {
		criterion = "gini"
	}
	return &DecisionTree{
		maxDepth:  maxDepth,
		minSample: minSample,
		criterion: criterion,
	}
}

// Fit 训练决策树模型。
func (dt *DecisionTree) Fit(points []*DTPoint, labels []int) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	dt.root = dt.buildTree(points, labels, 0)
}

// buildTree 递归构建决策树的函数。
// points: 当前节点对应的训练数据集。
// labels: 当前节点对应的标签集。
// depth: 当前节点的深度。
// 返回构建好的子树的根节点。
func (dt *DecisionTree) buildTree(points []*DTPoint, labels []int, depth int) *TreeNode {
	numSamples := len(points)

	// 终止条件检查
	if numSamples == 0 {
		return nil
	}

	// 1. 检查是否为纯净节点 (所有标签相同)
	firstLabel := labels[0]
	isPure := true
	labelCount := make(map[int]int)
	for _, l := range labels {
		labelCount[l]++
		if l != firstLabel {
			isPure = false
		}
	}

	if isPure || numSamples < dt.minSample || depth >= dt.maxDepth {
		maxCount := 0
		maxLabel := 0
		for l, count := range labelCount {
			if count > maxCount {
				maxCount = count
				maxLabel = l
			}
		}
		return &TreeNode{IsLeaf: true, Label: maxLabel}
	}

	// 2. 寻找最佳分裂点
	bestGain := -1.0
	bestFeature := -1
	bestThreshold := 0.0

	dim := len(points[0].Data)
	for feature := 0; feature < dim; feature++ {
		// 提取该特征的所有可能切分点
		values := make([]float64, numSamples)
		for i, p := range points {
			values[i] = p.Data[feature]
		}
		sort.Float64s(values)

		// 尝试切分
		for i := 0; i < numSamples-1; i++ {
			if values[i] == values[i+1] {
				continue
			}
			threshold := (values[i] + values[i+1]) / 2

			// 分割数据集
			leftLabels, rightLabels := make([]int, 0), make([]int, 0)
			for j, p := range points {
				if p.Data[feature] <= threshold {
					leftLabels = append(leftLabels, labels[j])
				} else {
					rightLabels = append(rightLabels, labels[j])
				}
			}

			if len(leftLabels) == 0 || len(rightLabels) == 0 {
				continue
			}

			gain := dt.calculateGain(labels, leftLabels, rightLabels)
			if gain > bestGain {
				bestGain = gain
				bestFeature = feature
				bestThreshold = threshold
			}
		}
	}

	// 如果找不到有效的分裂点，降级为叶子节点
	if bestFeature == -1 {
		maxLabel := 0
		maxCount := 0
		for l, count := range labelCount {
			if count > maxCount {
				maxCount = count
				maxLabel = l
			}
		}
		return &TreeNode{IsLeaf: true, Label: maxLabel}
	}

	// 3. 执行分裂
	var leftPoints, rightPoints []*DTPoint
	var leftL, rightL []int
	for i, p := range points {
		if p.Data[bestFeature] <= bestThreshold {
			leftPoints = append(leftPoints, p)
			leftL = append(leftL, labels[i])
		} else {
			rightPoints = append(rightPoints, p)
			rightL = append(rightL, labels[i])
		}
	}

	return &TreeNode{
		Feature:   bestFeature,
		Threshold: bestThreshold,
		Left:      dt.buildTree(leftPoints, leftL, depth+1),
		Right:     dt.buildTree(rightPoints, rightL, depth+1),
		IsLeaf:    false,
	}
}

// calculateGain 根据选择的标准计算增益。
func (dt *DecisionTree) calculateGain(parent, left, right []int) float64 {
	if dt.criterion == "entropy" {
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
	// 基尼增益：Gini(Parent) - Sum(Weight * Gini(Child))
	return parentGini - (leftWeight*dt.gini(left) + rightWeight*dt.gini(right))
}

func (dt *DecisionTree) gini(labels []int) float64 {
	if len(labels) == 0 {
		return 0
	}
	counts := make(map[int]int)
	for _, l := range labels {
		counts[l]++
	}

	impurity := 1.0
	for _, count := range counts {
		p := float64(count) / float64(len(labels))
		impurity -= p * p
	}
	return impurity
}

func (dt *DecisionTree) entropy(labels []int) float64 {
	if len(labels) == 0 {
		return 0
	}
	counts := make(map[int]int)
	for _, l := range labels {
		counts[l]++
	}
	var res float64
	for _, count := range counts {
		p := float64(count) / float64(len(labels))
		if p > 0 {
			res -= p * math.Log2(p)
		}
	}
	return res
}

// Predict 使用训练好的决策树对新的数据点进行预测。
// data: 待预测数据点的特征向量。
func (dt *DecisionTree) Predict(data []float64) int {
	dt.mu.RLock()         // 预测过程需要加读锁。
	defer dt.mu.RUnlock() // 确保函数退出时解锁。

	return dt.predictNode(dt.root, data) // 从根节点开始递归预测。
}

// predictNode 递归地在决策树中遍历，直到到达叶子节点并返回其标签。
func (dt *DecisionTree) predictNode(node *TreeNode, data []float64) int {
	if node.IsLeaf {
		return node.Label // 如果是叶子节点，则返回其预测标签。
	}

	// 根据当前节点的分裂特征和阈值，决定走向左子树还是右子树。
	if data[node.Feature] <= node.Threshold {
		return dt.predictNode(node.Left, data)
	}
	return dt.predictNode(node.Right, data)
}
