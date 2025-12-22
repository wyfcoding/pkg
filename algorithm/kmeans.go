package algorithm

import (
	"math"
	"sync"
)

// Point 结构体代表数据集中的一个数据点。
// 它是K-Means算法处理的基本单元。
type KMeansPoint struct {
	ID    uint64    // 数据点的唯一标识符。
	Data  []float64 // 数据点的特征向量，即多维坐标。
	Label int       // 数据点所属的簇标签（-1表示未分配或未训练）。
}

// KMeans 结构体实现了K均值聚类算法。
// 它用于将数据集划分为K个簇，每个簇由其质心（centroid）代表。
type KMeans struct {
	k         int            // 聚类的数量。
	points    []*KMeansPoint // 参与聚类的数据点集合。
	centroids [][]float64    // 存储K个簇的质心坐标。
	maxIter   int            // 算法的最大迭代次数，防止不收敛。
	tolerance float64        // 质心移动的容忍度，用于判断算法是否收敛。
	mu        sync.RWMutex   // 读写锁，用于在训练和预测时的并发安全。
}

// NewKMeans 创建并返回一个新的 KMeans 聚类器实例。
// k: 期望将数据分成的簇的数量。
// maxIter: K-Means算法的最大迭代次数。
// tolerance: 判断质心是否收敛的阈值。
func NewKMeans(k, maxIter int, tolerance float64) *KMeans {
	return &KMeans{
		k:         k,
		maxIter:   maxIter,
		tolerance: tolerance,
	}
}

// Fit 训练KMeans模型。
// points: 待聚类的数据点集合。
func (km *KMeans) Fit(points []*KMeansPoint) {
	km.mu.Lock()         // 训练过程需要加写锁。
	defer km.mu.Unlock() // 确保函数退出时解锁。

	km.points = points
	if len(points) == 0 {
		return // 如果没有数据点，则直接返回。
	}

	dim := len(points[0].Data) // 数据点的维度。
	km.centroids = make([][]float64, km.k)

	// 随机初始化K个质心。
	// 简单的初始化策略：从数据点中随机选择K个点作为初始质心。
	for i := 0; i < km.k; i++ {
		km.centroids[i] = make([]float64, dim)
		// 避免索引越界，取模操作。
		copy(km.centroids[i], points[i%len(points)].Data)
	}

	// 迭代优化过程：
	// 在maxIter次迭代内，重复“分配-更新”步骤，直到质心收敛或达到最大迭代次数。
	for iter := 0; iter < km.maxIter; iter++ {
		// 步骤1: 将每个数据点分配到最近的质心所属的簇。
		for _, p := range km.points {
			minDist := math.Inf(1) // 最小距离初始化为正无穷。
			minLabel := 0          // 最小距离对应的簇标签。
			for j, centroid := range km.centroids {
				// 计算数据点到每个质心的欧几里得距离。
				dist := km.euclideanDistance(p.Data, centroid)
				if dist < minDist {
					minDist = dist
					minLabel = j // 更新最近的簇标签。
				}
			}
			p.Label = minLabel // 将数据点分配到最近的簇。
		}

		// 步骤2: 更新每个簇的质心。
		// 新的质心是该簇内所有数据点的平均值。
		oldCentroids := make([][]float64, km.k) // 备份旧质心，用于判断收敛。
		for i := range oldCentroids {
			oldCentroids[i] = make([]float64, dim)
			copy(oldCentroids[i], km.centroids[i])
		}

		for j := 0; j < km.k; j++ { // 遍历每个簇。
			count := 0 // 统计当前簇的数据点数量。
			// 将当前质心坐标清零，准备重新计算平均值。
			for d := range dim {
				km.centroids[j][d] = 0
			}

			// 累加当前簇内所有数据点的特征向量。
			for _, p := range km.points {
				if p.Label == j {
					for d := range dim {
						km.centroids[j][d] += p.Data[d]
					}
					count++
				}
			}

			// 如果簇不为空，则计算新的质心（平均值）。
			if count > 0 {
				for d := range dim {
					km.centroids[j][d] /= float64(count)
				}
			}
		}

		// 检查收敛：比较新旧质心之间的距离。
		converged := true
		for j := 0; j < km.k; j++ {
			// 如果任何一个质心移动的距离超过容忍度，则认为未收敛。
			if km.euclideanDistance(oldCentroids[j], km.centroids[j]) > km.tolerance {
				converged = false
				break
			}
		}

		if converged {
			break // 如果所有质心都已收敛，则提前结束迭代。
		}
	}
}

// Predict 预测新数据点所属的簇标签。
// data: 待预测数据点的特征向量。
// 返回数据点所属的簇标签。
func (km *KMeans) Predict(data []float64) int {
	km.mu.RLock()         // 预测过程只需要读锁。
	defer km.mu.RUnlock() // 确保函数退出时解锁。

	minDist := math.Inf(1) // 最小距离初始化为正无穷。
	minLabel := 0          // 最小距离对应的簇标签。

	// 找到距离新数据点最近的质心。
	for j, centroid := range km.centroids {
		dist := km.euclideanDistance(data, centroid)
		if dist < minDist {
			minDist = dist
			minLabel = j
		}
	}

	return minLabel // 返回最近质心所属的簇标签。
}

// euclideanDistance 计算两个多维向量之间的欧几里得距离。
func (km *KMeans) euclideanDistance(a, b []float64) float64 {
	var sum float64
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff // 累加平方差。
	}
	return math.Sqrt(sum) // 返回平方和的平方根。
}
