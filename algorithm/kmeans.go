package algorithm

import (
	"math"
	"math/rand/v2"
	"runtime"
	"sync"
)

// KMeansPoint 结构体代表数据集中的一个数据点。
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
	n := len(points)
	if n == 0 {
		return
	}

	dim := len(points[0].Data)
	// 确保 centroids 空间已分配
	if len(km.centroids) != km.k {
		km.centroids = make([][]float64, km.k)
		for i := range km.centroids {
			km.centroids[i] = make([]float64, dim)
		}
	}

	// 真实化实现：K-Means++ 初始化策略
	// 1. 随机选取第一个点作为质心
	firstIdx := rand.IntN(n)
	copy(km.centroids[0], points[firstIdx].Data)

	// 复用 distances 切片
	distances := make([]float64, n)

	// 2. 选取剩余的 k-1 个质心
	for i := 1; i < km.k; i++ {
		// 计算每个点到当前已选质心集合的最小距离的平方 D(x)^2
		var totalDistSq float64
		for idx, p := range points {
			minDistSq := math.MaxFloat64
			for j := 0; j < i; j++ {
				// 使用平方距离
				d := km.euclideanDistanceSquared(p.Data, km.centroids[j])
				if d < minDistSq {
					minDistSq = d
				}
			}
			distances[idx] = minDistSq
			totalDistSq += distances[idx]
		}

		// 基于概率分布选取下一个质心 (轮盘赌算法)
		target := rand.Float64() * totalDistSq
		var currentSum float64
		chosen := false
		for idx, dSq := range distances {
			currentSum += dSq
			if currentSum >= target {
				copy(km.centroids[i], points[idx].Data)
				chosen = true
				break
			}
		}
		// 容错处理
		if !chosen {
			copy(km.centroids[i], points[n-1].Data)
		}
	}

	// 准备并发相关参数
	numWorkers := runtime.GOMAXPROCS(0)
	if n < 1000 {
		numWorkers = 1
	}
	chunkSize := (n + numWorkers - 1) / numWorkers

	// 全局累加器 (Main thread aggregation)
	newCentroids := make([][]float64, km.k)
	for i := range newCentroids {
		newCentroids[i] = make([]float64, dim)
	}
	counts := make([]int, km.k)

	// 局部累加器池 (Worker thread accumulation)
	// [workerID][clusterID][dimension]
	localCentroids := make([][][]float64, numWorkers)
	localCounts := make([][]int, numWorkers)
	for w := 0; w < numWorkers; w++ {
		localCentroids[w] = make([][]float64, km.k)
		for k := 0; k < km.k; k++ {
			localCentroids[w][k] = make([]float64, dim)
		}
		localCounts[w] = make([]int, km.k)
	}

	toleranceSq := km.tolerance * km.tolerance

	// 迭代优化过程 (分配-更新)...
	for iter := 0; iter < km.maxIter; iter++ {
		// 步骤0: 重置累加器 (Global & Local)
		for i := 0; i < km.k; i++ {
			counts[i] = 0
			for d := 0; d < dim; d++ {
				newCentroids[i][d] = 0
			}
		}
		for w := 0; w < numWorkers; w++ {
			for i := 0; i < km.k; i++ {
				localCounts[w][i] = 0
				for d := 0; d < dim; d++ {
					localCentroids[w][i][d] = 0
				}
			}
		}

		// 步骤1: 并行分配 (Assignment Step)
		var wg sync.WaitGroup
		wg.Add(numWorkers)

		for w := 0; w < numWorkers; w++ {
			start := w * chunkSize
			end := start + chunkSize
			if end > n {
				end = n
			}

			go func(workerID int, startIdx, endIdx int) {
				defer wg.Done()
				myCounts := localCounts[workerID]
				myCentroids := localCentroids[workerID]

				for i := startIdx; i < endIdx; i++ {
					p := points[i]
					minDistSq := math.MaxFloat64
					minLabel := 0

					// 寻找最近质心
					for j, centroid := range km.centroids {
						distSq := km.euclideanDistanceSquared(p.Data, centroid)
						if distSq < minDistSq {
							minDistSq = distSq
							minLabel = j
						}
					}
					p.Label = minLabel

					// 累加到本地缓存
					myCounts[minLabel]++
					for d := 0; d < dim; d++ {
						myCentroids[minLabel][d] += p.Data[d]
					}
				}
			}(w, start, end)
		}
		wg.Wait()

		// 聚合所有 Worker 的结果到全局累加器
		for w := 0; w < numWorkers; w++ {
			for i := 0; i < km.k; i++ {
				counts[i] += localCounts[w][i]
				for d := 0; d < dim; d++ {
					newCentroids[i][d] += localCentroids[w][i][d]
				}
			}
		}

		// 步骤2: 更新 (Update Step)
		// 计算平均值得到新质心
		for j := 0; j < km.k; j++ {
			if counts[j] > 0 {
				invCount := 1.0 / float64(counts[j])
				for d := 0; d < dim; d++ {
					newCentroids[j][d] *= invCount
				}
			} else {
				// 如果簇为空，保持原质心防止漂移
				copy(newCentroids[j], km.centroids[j])
			}
		}

		// 步骤3: 检查收敛并交换
		converged := true
		for j := 0; j < km.k; j++ {
			if km.euclideanDistanceSquared(newCentroids[j], km.centroids[j]) > toleranceSq {
				converged = false
			}
			// 更新当前质心
			km.centroids[j], newCentroids[j] = newCentroids[j], km.centroids[j]
		}

		if converged {
			break
		}
	}
}

// Predict 预测新数据点所属的簇标签。
func (km *KMeans) Predict(data []float64) int {
	km.mu.RLock()         // 预测过程只需要读锁。
	defer km.mu.RUnlock() // 确保函数退出时解锁。

	minDistSq := math.MaxFloat64 // 最小距离初始化为正无穷。
	minLabel := 0                // 最小距离对应的簇标签。

	// 找到距离新数据点最近的质心。
	for j, centroid := range km.centroids {
		distSq := km.euclideanDistanceSquared(data, centroid)
		if distSq < minDistSq {
			minDistSq = distSq
			minLabel = j
		}
	}

	return minLabel // 返回最近质心所属的簇标签。
}

// euclideanDistanceSquared 计算两个多维向量之间的欧几里得距离的平方。
// 避免了 Sqrt 运算，极大提升比较效率。
func (km *KMeans) euclideanDistanceSquared(a, b []float64) float64 {
	var sum float64
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff // 累加平方差。
	}
	return sum
}
