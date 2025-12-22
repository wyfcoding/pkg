package algorithm

import (
	"math"
	"math/rand/v2" // 使用 Go 1.22+ 提供的新的 rand/v2 包。
	"sync"
)

// RandomForest 结构体实现了随机森林算法。
// 随机森林是一种集成学习方法，通过构建大量的决策树并聚合它们的预测结果，
// 以提高预测的准确性和稳定性，常用于分类和回归任务。
type RandomForest struct {
	trees    []*DecisionTree // 包含在森林中的所有决策树实例。
	numTrees int             // 森林中决策树的数量。
	mu       sync.RWMutex    // 读写锁，用于在训练和预测时的并发安全。
}

// NewRandomForest 创建并返回一个新的 RandomForest 实例。
// numTrees: 森林中要构建的决策树的数量。
func NewRandomForest(numTrees int) *RandomForest {
	return &RandomForest{
		trees:    make([]*DecisionTree, numTrees),
		numTrees: numTrees,
	}
}

// Fit 训练随机森林模型。
// 在训练过程中，会使用Bootstrap Aggregating（Bagging）方法对原始数据集进行采样，
// 然后为每个采样数据集训练一个独立的决策树。
// 应用场景：复杂用户行为预测、欺诈检测、推荐系统等。
// points: 训练数据集的特征点切片。
// labels: 每个数据点对应的真实标签。
func (rf *RandomForest) Fit(points []*DTPoint, labels []int) {
	rf.mu.Lock()         // 训练过程需要加写锁。
	defer rf.mu.Unlock() // 确保函数退出时解锁。

	// 为森林中的每一棵树进行训练。
	for i := 0; i < rf.numTrees; i++ {
		// 步骤1: Bootstrap采样 (有放回随机采样)。
		// 从原始数据集中有放回地随机抽取与原始数据集相同数量的样本，
		// 形成一个Bootstrap数据集。
		bootstrapPoints := make([]*DTPoint, len(points))
		bootstrapLabels := make([]int, len(labels))

		for j := range points {
			// 随机选择一个索引。
			idx := int(math.Floor(rand.Float64() * float64(len(points))))
			bootstrapPoints[j] = points[idx]
			bootstrapLabels[j] = labels[idx]
		}

		// 步骤2: 训练决策树。
		// 为每个Bootstrap数据集训练一个决策树。
		// 这里假设 DecisionTree 已经存在于 algorithm 包中。
		// DecisionTree 的参数（例如 maxDepth=10, minSample=5）需要根据具体问题调整。
		tree := NewDecisionTree(10, 5)             // 创建一个新的决策树实例。
		tree.Fit(bootstrapPoints, bootstrapLabels) // 使用Bootstrap样本训练决策树。
		rf.trees[i] = tree                         // 将训练好的决策树添加到森林中。
	}
}

// Predict 使用训练好的随机森林模型对新的数据点进行预测。
// 预测结果通过“投票”机制决定：森林中所有决策树的预测结果中，得票最多的标签即为最终预测结果。
// data: 待预测数据点的特征向量。
// 返回预测的类别标签。
func (rf *RandomForest) Predict(data []float64) int {
	rf.mu.RLock()         // 预测过程只需要读锁。
	defer rf.mu.RUnlock() // 确保函数退出时解锁。

	votes := make(map[int]int) // 记录每个标签的得票数。
	// 遍历森林中的所有决策树，获取每个树的预测结果。
	for _, tree := range rf.trees {
		label := tree.Predict(data) // 获取单个决策树的预测结果。
		votes[label]++              // 增加对应标签的得票数。
	}

	maxVotes := 0 // 记录最高的得票数。
	maxLabel := 0 // 记录得票最高的标签。
	// 统计得票最多的标签。
	for label, count := range votes {
		if count > maxVotes {
			maxVotes = count
			maxLabel = label
		}
	}

	return maxLabel // 返回得票最多的标签作为最终预测结果。
}
