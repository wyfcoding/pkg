// Package algorithm 提供了 K-Means 聚类核算法。
package ml

import (
	crypto_rand "crypto/rand"
	"encoding/binary"
	"math"
	"time"
)

// KMeans K-Means 聚类结构体。
type KMeans struct {
	data      [][]float64
	centroids [][]float64
	clusters  []int
	k         int
}

// NewKMeans 创建 K-Means 实例。
func NewKMeans(k int, data [][]float64) *KMeans {
	return &KMeans{
		k:    k,
		data: data,
	}
}

// Fit 执行聚类训练。
func (km *KMeans) Fit(maxIter int) {
	km.initializeCentroidsPlusPlus()

	for range maxIter {
		if !km.updateClusters() {
			break
		}
		km.updateCentroids()
	}
}

// initializeCentroidsPlusPlus 使用 K-Means++ 算法初始化质心。
// 优化：使用 crypto/rand 确保初始化质量与安全性。
func (km *KMeans) initializeCentroidsPlusPlus() {
	n := len(km.data)
	if n == 0 {
		return
	}

	km.centroids = make([][]float64, 0, km.k)

	// 随机选择第一个质心.
	var b [8]byte
	if _, err := crypto_rand.Read(b[:]); err != nil {
		ts := time.Now().UnixNano()
		if ts < 0 {
			ts = -ts
		}
		binary.LittleEndian.PutUint64(b[:], uint64(ts)) //nolint:gosec // ts >= 0 已保证。
	}
	val := binary.LittleEndian.Uint64(b[:]) % uint64(n)
	var firstIdx int
	if val <= uint64(math.MaxInt) {
		firstIdx = int(val)
	} else {
		firstIdx = 0
	}
	km.centroids = append(km.centroids, km.data[firstIdx])

	for len(km.centroids) < km.k {
		distances := make([]float64, n)
		totalDist := 0.0

		for i, point := range km.data {
			minDist := math.MaxFloat64
			for _, centroid := range km.centroids {
				dist := km.euclideanDistance(point, centroid)
				if dist < minDist {
					minDist = dist
				}
			}
			distances[i] = minDist * minDist
			totalDist += distances[i]
		}

		// 轮盘赌选择下一个质心.
		if _, err := crypto_rand.Read(b[:]); err != nil {
			ts := time.Now().UnixNano()
			if ts < 0 {
				ts = -ts
			}
			binary.LittleEndian.PutUint64(b[:], uint64(ts)) //nolint:gosec // ts >= 0 已保证。
		}
		r := (float64(binary.LittleEndian.Uint64(b[:])) / float64(math.MaxUint64)) * totalDist
		sum := 0.0
		for i, d := range distances {
			sum += d
			if sum >= r {
				km.centroids = append(km.centroids, km.data[i])
				break
			}
		}
	}
}

func (km *KMeans) updateClusters() bool {
	changed := false
	km.clusters = make([]int, len(km.data))

	for i, point := range km.data {
		minDist := math.MaxFloat64
		bestCluster := 0

		for j, centroid := range km.centroids {
			dist := km.euclideanDistance(point, centroid)
			if dist < minDist {
				minDist = dist
				bestCluster = j
			}
		}

		if km.clusters[i] != bestCluster {
			km.clusters[i] = bestCluster
			changed = true
		}
	}

	return changed
}

func (km *KMeans) updateCentroids() {
	newCentroids := make([][]float64, km.k)
	counts := make([]int, km.k)

	dims := len(km.data[0])
	for i := range km.k {
		newCentroids[i] = make([]float64, dims)
	}

	for i, clusterID := range km.clusters {
		for j, val := range km.data[i] {
			newCentroids[clusterID][j] += val
		}
		counts[clusterID]++
	}

	for i := range km.k {
		if counts[i] > 0 {
			for j := range dims {
				newCentroids[i][j] /= float64(counts[i])
			}
		}
	}

	km.centroids = newCentroids
}

func (km *KMeans) euclideanDistance(p1, p2 []float64) float64 {
	sum := 0.0
	for i := range p1 {
		diff := p1[i] - p2[i]
		sum += diff * diff
	}
	return math.Sqrt(sum)
}

// Predict 预测数据点的聚类 ID。
func (km *KMeans) Predict(point []float64) int {
	minDist := math.MaxFloat64
	bestCluster := 0

	for i, centroid := range km.centroids {
		dist := km.euclideanDistance(point, centroid)
		if dist < minDist {
			minDist = dist
			bestCluster = i
		}
	}

	return bestCluster
}

// GetAssignments 返回每个点的聚类归属（索引对应输入数据的顺序）。
func (km *KMeans) GetAssignments() []int {
	return km.clusters
}
