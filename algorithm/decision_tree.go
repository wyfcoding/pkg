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
// 它包含了构建树的参数和根节点。
type DecisionTree struct {
	root      *TreeNode    // 决策树的根节点。
	maxDepth  int          // 树的最大深度，用于防止过拟合。
	minSample int          // 叶子节点包含的最小样本数，用于防止过拟合。
	mu        sync.RWMutex // 读写锁，用于在训练和预测时的并发安全。
}

// NewDecisionTree 创建并返回一个新的 DecisionTree 实例。
// maxDepth: 树允许的最大深度。
// minSample: 一个节点要继续分裂所需的最小样本数，否则将成为叶子节点。
func NewDecisionTree(maxDepth, minSample int) *DecisionTree {
	return &DecisionTree{
		maxDepth:  maxDepth,
		minSample: minSample,
	}
}

// Fit 训练决策树模型。
// points: 训练数据集的特征点切片。
// labels: 每个数据点对应的真实标签。
func (dt *DecisionTree) Fit(points []*DTPoint, labels []int) {
	dt.mu.Lock()         // 训练过程需要加写锁。
	defer dt.mu.Unlock() // 确保函数退出时解锁。

	dt.root = dt.buildTree(points, labels, 0) // 从根节点开始递归构建树。
}

// buildTree 递归构建决策树的函数。
// points: 当前节点对应的训练数据集。
// labels: 当前节点对应的标签集。
// depth: 当前节点的深度。
// 返回构建好的子树的根节点。
func (dt *DecisionTree) buildTree(points []*DTPoint, labels []int, depth int) *TreeNode {
	// 终止条件：
	// 1. 样本数小于最小样本数阈值。
	// 2. 达到最大深度。
	// 3. 所有样本的标签都相同（纯净节点，此处未显式检查，隐含在熵的计算中）。
	if len(points) < dt.minSample || depth >= dt.maxDepth {
		// 如果达到终止条件，则创建叶子节点。
		labelCount := make(map[int]int)
		for _, label := range labels {
			labelCount[label]++
		}

		maxCount := 0
		maxLabel := 0
		// 找出当前节点中出现次数最多的标签作为叶子节点的预测值。
		for label, count := range labelCount {
			if count > maxCount {
				maxCount = count
				maxLabel = label
			}
		}

		return &TreeNode{
			IsLeaf: true,
			Label:  maxLabel,
		}
	}

	// 寻找最佳分裂点：
	// 目标是找到一个特征和一个阈值，使得分裂后子节点的纯度最高（信息增益最大）。
	bestGain := 0.0      // 记录当前找到的最佳信息增益。
	bestFeature := 0     // 记录最佳分裂特征的索引。
	bestThreshold := 0.0 // 记录最佳分裂阈值。

	dim := len(points[0].Data) // 数据特征的维度。
	for feature := 0; feature < dim; feature++ {
		// 获取当前特征的所有值，并排序。
		values := make([]float64, len(points))
		for i, p := range points {
			values[i] = p.Data[feature]
		}
		sort.Float64s(values)

		// 尝试所有可能的中间点作为分裂阈值。
		for i := 0; i < len(values)-1; i++ {
			threshold := (values[i] + values[i+1]) / 2 // 取相邻两个值的中间点作为阈值。

			// 根据当前特征和阈值将数据集分裂为左右两部分。
			leftPoints := make([]*DTPoint, 0)
			leftLabels := make([]int, 0)
			rightPoints := make([]*DTPoint, 0)
			rightLabels := make([]int, 0)

			for j, p := range points {
				if p.Data[feature] <= threshold {
					leftPoints = append(leftPoints, p)
					leftLabels = append(leftLabels, labels[j])
				} else {
					rightPoints = append(rightPoints, p)
					rightLabels = append(rightLabels, labels[j])
				}
			}

			// 如果分裂后，任一边为空，则此分裂无效。
			if len(leftPoints) == 0 || len(rightPoints) == 0 {
				continue
			}

			// 计算当前分裂的信息增益。
			gain := dt.calculateGain(labels, leftLabels, rightLabels)
			// 如果找到更大的信息增益，则更新最佳分裂点。
			if gain > bestGain {
				bestGain = gain
				bestFeature = feature
				bestThreshold = threshold
			}
		}
	}

	// 根据找到的最佳分裂点，实际分裂数据集。
	leftPoints := make([]*DTPoint, 0)
	leftLabels := make([]int, 0)
	rightPoints := make([]*DTPoint, 0)
	rightLabels := make([]int, 0)

	for i, p := range points {
		if p.Data[bestFeature] <= bestThreshold {
			leftPoints = append(leftPoints, p)
			leftLabels = append(leftLabels, labels[i])
		} else {
			rightPoints = append(rightPoints, p)
			rightLabels = append(rightLabels, labels[i])
		}
	}

	// 创建当前内部节点，并递归构建其左右子树。
	return &TreeNode{
		Feature:   bestFeature,
		Threshold: bestThreshold,
		Left:      dt.buildTree(leftPoints, leftLabels, depth+1),   // 递归构建左子树。
		Right:     dt.buildTree(rightPoints, rightLabels, depth+1), // 递归构建右子树。
		IsLeaf:    false,                                           // 标记为内部节点。
	}
}

// calculateGain 计算信息增益。
// 信息增益 = 父节点熵 - (左子节点熵 * 左子节点权重 + 右子节点熵 * 右子节点权重)。
func (dt *DecisionTree) calculateGain(parent, left, right []int) float64 {
	parentEntropy := dt.entropy(parent) // 计算父节点的熵。

	// 计算左右子节点的权重（占父节点总样本数的比例）。
	leftWeight := float64(len(left)) / float64(len(parent))
	rightWeight := float64(len(right)) / float64(len(parent))

	// 计算分裂后的加权熵。
	childEntropy := leftWeight*dt.entropy(left) + rightWeight*dt.entropy(right)

	return parentEntropy - childEntropy // 返回信息增益。
}

// entropy 计算给定标签集合的熵（信息论中的不确定性度量）。
func (dt *DecisionTree) entropy(labels []int) float64 {
	if len(labels) == 0 {
		return 0 // 空集的熵为0。
	}

	labelCount := make(map[int]int)
	for _, label := range labels {
		labelCount[label]++ // 统计每个标签的出现次数。
	}

	var entropy float64
	for _, count := range labelCount {
		p := float64(count) / float64(len(labels)) // 计算当前标签的概率。
		if p > 0 {
			entropy -= p * math.Log2(p) // 根据熵的公式累加。
		}
	}

	return entropy
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
