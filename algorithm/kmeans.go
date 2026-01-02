package algorithm

import (
	"math"
	"math/rand/v2"
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
func (km *KMeans) Fit(points []*KMeansPoint) {
	km.mu.Lock()
	defer km.mu.Unlock()

	km.points = points
	if len(points) == 0 {
		return
	}

	dim := len(points[0].Data)
	km.centroids = make([][]float64, km.k)

	// 真实化实现：K-Means++ 初始化策略
	// 1. 随机选取第一个点作为质心
	firstIdx := rand.IntN(len(points))
	km.centroids[0] = make([]float64, dim)
	copy(km.centroids[0], points[firstIdx].Data)

	// 2. 选取剩余的 k-1 个质心
	for i := 1; i < km.k; i++ {
		// 计算每个点到当前已选质心集合的最小距离的平方 D(x)^2
		distances := make([]float64, len(points))
		var totalDistSq float64
		for idx, p := range points {
			minDist := math.MaxFloat64
			for j := 0; j < i; j++ {
				d := km.euclideanDistance(p.Data, km.centroids[j])
				if d < minDist {
					minDist = d
				}
			}
			distances[idx] = minDist * minDist
			totalDistSq += distances[idx]
		}

		// 基于概率分布选取下一个质心 (轮盘赌算法)
		target := rand.Float64() * totalDistSq
		var currentSum float64
		for idx, dSq := range distances {
			currentSum += dSq
			if currentSum >= target {
				km.centroids[i] = make([]float64, dim)
				copy(km.centroids[i], points[idx].Data)
				break
			}
		}
		// 容错处理：如果没选到则选最后一个
		if km.centroids[i] == nil {
			km.centroids[i] = make([]float64, dim)
			copy(km.centroids[i], points[len(points)-1].Data)
		}
	}

	// 迭代优化过程 (分配-更新)...
	for iter := 0; iter < km.maxIter; iter++ {
		// 步骤1: 分配
		for _, p := range km.points {
			minDist := math.Inf(1)
			minLabel := 0
			for j, centroid := range km.centroids {
				dist := km.euclideanDistance(p.Data, centroid)
				if dist < minDist {
					minDist = dist
					minLabel = j
				}
			}
			p.Label = minLabel
		}

		// 步骤2: 更新
		oldCentroids := make([][]float64, km.k)
		for i := range oldCentroids {
			oldCentroids[i] = make([]float64, dim)
			copy(oldCentroids[i], km.centroids[i])
		}

		for j := 0; j < km.k; j++ {
			count := 0
			for d := range dim {
				km.centroids[j][d] = 0
			}

			for _, p := range km.points {
				if p.Label == j {
					for d := range dim {
						km.centroids[j][d] += p.Data[d]
					}
					count++
				}
			}

			if count > 0 {
				for d := range dim {
					km.centroids[j][d] /= float64(count)
				}
			}
		}

		converged := true
		for j := 0; j < km.k; j++ {
			if km.euclideanDistance(oldCentroids[j], km.centroids[j]) > km.tolerance {
				converged = false
				break
			}
		}
		if converged {
			break
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
