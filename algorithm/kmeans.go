package algorithm

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"runtime"
	"sync"
	"time"
)

// KMeansPoint 结构体代表数据集中的一个数据点.
type KMeansPoint struct {
	Data  []float64
	ID    uint64
	Label int
}

// KMeans 结构体实现了K均值聚类算法.
type KMeans struct {
	centroids [][]float64
	points    []*KMeansPoint
	mu        sync.RWMutex
	tolerance float64
	k         int
	maxIter   int
}

// NewKMeans 创建并返回一个新的 KMeans 聚类器实例.
func NewKMeans(k, maxIter int, tolerance float64) *KMeans {
	return &KMeans{
		k:         k,
		maxIter:   maxIter,
		tolerance: tolerance,
		mu:        sync.RWMutex{},
		points:    nil,
		centroids: nil,
	}
}

// Fit 训练KMeans模型.
func (km *KMeans) Fit(points []*KMeansPoint) {
	km.mu.Lock()
	defer km.mu.Unlock()

	km.points = points
	n := len(points)
	if n == 0 {
		return
	}

	dim := len(points[0].Data)
	km.ensureCentroids(dim)
	km.initializeCentroidsPlusPlus(points)

	numWorkers := runtime.GOMAXPROCS(0)
	const minNForParallel = 1000
	if n < minNForParallel {
		numWorkers = 1
	}

	km.runIterations(points, numWorkers)
}

func (km *KMeans) ensureCentroids(dim int) {
	if len(km.centroids) != km.k {
		km.centroids = make([][]float64, km.k)
		for i := range km.k {
			km.centroids[i] = make([]float64, dim)
		}
	}
}

func (km *KMeans) initializeCentroidsPlusPlus(points []*KMeansPoint) {
	n := len(points)
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		binary.LittleEndian.PutUint64(b[:], uint64(time.Now().UnixNano()))
	}
	firstIdx := int(binary.LittleEndian.Uint64(b[:]) % uint64(n))
	copy(km.centroids[0], points[firstIdx].Data)

	distances := make([]float64, n)
	for i := 1; i < km.k; i++ {
		var totalDistSq float64
		for idx, p := range points {
			minDistSq := math.MaxFloat64
			for j := range i {
				d := km.euclideanDistanceSquared(p.Data, km.centroids[j])
				if d < minDistSq {
					minDistSq = d
				}
			}
			distances[idx] = minDistSq
			totalDistSq += distances[idx]
		}

		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			binary.LittleEndian.PutUint64(b[:], uint64(time.Now().UnixNano()))
		}
		rv := float64(binary.LittleEndian.Uint64(b[:])) / float64(math.MaxUint64)
		target := rv * totalDistSq
		km.selectNextCentroid(points, distances, target, i)
	}
}

func (km *KMeans) selectNextCentroid(points []*KMeansPoint, distances []float64, target float64, centroidIdx int) {
	var currentSum float64
	chosen := false
	for idx, dSq := range distances {
		currentSum += dSq
		if currentSum >= target {
			copy(km.centroids[centroidIdx], points[idx].Data)
			chosen = true
			break
		}
	}
	if !chosen {
		copy(km.centroids[centroidIdx], points[len(points)-1].Data)
	}
}

func (km *KMeans) runIterations(points []*KMeansPoint, numWorkers int) {
	n := len(points)
	dim := len(points[0].Data)
	chunkSize := (n + numWorkers - 1) / numWorkers

	newCentroids := make([][]float64, km.k)
	for i := range km.k {
		newCentroids[i] = make([]float64, dim)
	}
	counts := make([]int, km.k)

	localCentroids := make([][][]float64, numWorkers)
	localCounts := make([][]int, numWorkers)
	for w := range numWorkers {
		localCentroids[w] = make([][]float64, km.k)
		for k := range km.k {
			localCentroids[w][k] = make([]float64, dim)
		}
		localCounts[w] = make([]int, km.k)
	}

	toleranceSq := km.tolerance * km.tolerance
	for range km.maxIter {
		km.resetAggregators(newCentroids, counts, localCentroids, localCounts)
		km.performAssignment(points, numWorkers, chunkSize, localCentroids, localCounts)
		km.aggregateResults(newCentroids, counts, localCentroids, localCounts)

		if km.updateAndCheckConvergence(newCentroids, counts, toleranceSq) {
			break
		}
	}
}

func (km *KMeans) resetAggregators(newCentroids [][]float64, counts []int, localCentroids [][][]float64, localCounts [][]int) {
	dim := len(newCentroids[0])
	for i := range km.k {
		counts[i] = 0
		for d := range dim {
			newCentroids[i][d] = 0
		}
	}
	for w := range localCounts {
		for i := range km.k {
			localCounts[w][i] = 0
			for d := range dim {
				localCentroids[w][i][d] = 0
			}
		}
	}
}

func (km *KMeans) performAssignment(points []*KMeansPoint, numWorkers, chunkSize int, localCentroids [][][]float64, localCounts [][]int) {
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	n := len(points)
	dim := len(points[0].Data)

	for w := range numWorkers {
		start := w * chunkSize
		end := min(start+chunkSize, n)

		go func(workerID, startIdx, endIdx int) {
			defer wg.Done()
			myCounts := localCounts[workerID]
			myCentroids := localCentroids[workerID]

			for i := startIdx; i < endIdx; i++ {
				p := points[i]
				minDistSq := math.MaxFloat64
				minLabel := 0

				for j, centroid := range km.centroids {
					distSq := km.euclideanDistanceSquared(p.Data, centroid)
					if distSq < minDistSq {
						minDistSq = distSq
						minLabel = j
					}
				}
				p.Label = minLabel
				myCounts[minLabel]++
				for d := range dim {
					myCentroids[minLabel][d] += p.Data[d]
				}
			}
		}(w, start, end)
	}
	wg.Wait()
}

func (km *KMeans) aggregateResults(newCentroids [][]float64, counts []int, localCentroids [][][]float64, localCounts [][]int) {
	for w := range localCounts {
		for i := range km.k {
			counts[i] += localCounts[w][i]
			for d := range len(newCentroids[0]) {
				newCentroids[i][d] += localCentroids[w][i][d]
			}
		}
	}
}

func (km *KMeans) updateAndCheckConvergence(newCentroids [][]float64, counts []int, toleranceSq float64) bool {
	dim := len(newCentroids[0])
	for j := range km.k {
		if counts[j] > 0 {
			invCount := 1.0 / float64(counts[j])
			for d := range dim {
				newCentroids[j][d] *= invCount
			}
		} else {
			copy(newCentroids[j], km.centroids[j])
		}
	}

	converged := true
	for j := range km.k {
		if km.euclideanDistanceSquared(newCentroids[j], km.centroids[j]) > toleranceSq {
			converged = false
		}
		km.centroids[j], newCentroids[j] = newCentroids[j], km.centroids[j]
	}
	return converged
}

// Predict 预测新数据点所属的簇标签.
func (km *KMeans) Predict(data []float64) int {
	km.mu.RLock()
	defer km.mu.RUnlock()

	minDistSq := math.MaxFloat64
	minLabel := 0

	for j, centroid := range km.centroids {
		distSq := km.euclideanDistanceSquared(data, centroid)
		if distSq < minDistSq {
			minDistSq = distSq
			minLabel = j
		}
	}

	return minLabel
}

func (km *KMeans) euclideanDistanceSquared(a, b []float64) float64 {
	var sum float64
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return sum
}
