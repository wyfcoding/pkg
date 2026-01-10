package algorithm

import (
	"errors"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync/atomic"
	"time"
)

var (
	// ErrInvalidEpsilonDelta 无效的 epsilon 或 delta 参数.
	ErrInvalidEpsilonDelta = errors.New("invalid epsilon or delta")
	// ErrDimensionMismatch 维度不匹配，无法合并.
	ErrDimensionMismatch = errors.New("cannot merge CountMinSketch with different dimensions")
)

// CountMinSketch 高性能概率数据结构.
type CountMinSketch struct {
	matrix []uint64
	seeds  []uint32
	width  uint
	depth  uint
	count  uint64
}

// NewCountMinSketch 创建 CMS.
func NewCountMinSketch(epsilon, delta float64) (*CountMinSketch, error) {
	if epsilon <= 0 || epsilon >= 1 || delta <= 0 || delta >= 1 {
		return nil, ErrInvalidEpsilonDelta
	}

	width := uint(math.Ceil(math.E / epsilon))
	depth := uint(math.Ceil(math.Log(1 / delta)))

	slog.Info("CountMinSketch initialized", "epsilon", epsilon, "delta", delta, "width", width, "depth", depth)

	return &CountMinSketch{
		width:  width,
		depth:  depth,
		matrix: make([]uint64, depth*width),
		seeds:  generateSeeds(depth),
		count:  0,
	}, nil
}

func generateSeeds(depth uint) []uint32 {
	seeds := make([]uint32, depth)
	for i := range seeds {
		seeds[i] = rand.Uint32()
	}

	return seeds
}

// Add 原子增加元素的计数.
func (cms *CountMinSketch) Add(data []byte, count uint64) {
	atomic.AddUint64(&cms.count, count)
	h1, h2 := getCMSHashes(data)

	for i := range cms.depth {
		index := (uint(h1) + uint(h2)*i + uint(cms.seeds[i])) % cms.width
		atomic.AddUint64(&cms.matrix[i*cms.width+index], count)
	}
}

// AddString 增加字符串元素的计数.
func (cms *CountMinSketch) AddString(key string, count uint64) {
	cms.Add([]byte(key), count)
}

// Estimate 估算频率.
func (cms *CountMinSketch) Estimate(data []byte) uint64 {
	minCount := uint64(math.MaxUint64)
	h1, h2 := getCMSHashes(data)

	for i := range cms.depth {
		index := (uint(h1) + uint(h2)*i + uint(cms.seeds[i])) % cms.width
		count := atomic.LoadUint64(&cms.matrix[i*cms.width+index])
		if count < minCount {
			minCount = count
		}
	}

	return minCount
}

// EstimateString 估算字符串频率.
func (cms *CountMinSketch) EstimateString(key string) uint64 {
	return cms.Estimate([]byte(key))
}

// Decay 衰减机制：将所有计数值减半.
func (cms *CountMinSketch) Decay() {
	start := time.Now()
	for i := range cms.matrix {
		val := atomic.LoadUint64(&cms.matrix[i])
		if val > 0 {
			atomic.StoreUint64(&cms.matrix[i], val/2)
		}
	}

	atomic.StoreUint64(&cms.count, atomic.LoadUint64(&cms.count)/2)
	slog.Info("CountMinSketch decay completed", "duration", time.Since(start))
}

// Reset 重置所有计数.
func (cms *CountMinSketch) Reset() {
	for i := range cms.matrix {
		atomic.StoreUint64(&cms.matrix[i], 0)
	}

	atomic.StoreUint64(&cms.count, 0)
}

// TotalCount 返回总的添加次数.
func (cms *CountMinSketch) TotalCount() uint64 {
	return atomic.LoadUint64(&cms.count)
}

// Merge 合并另一个 CMS.
func (cms *CountMinSketch) Merge(other *CountMinSketch) error {
	if cms.width != other.width || cms.depth != other.depth {
		return ErrDimensionMismatch
	}

	for i := range cms.matrix {
		otherVal := atomic.LoadUint64(&other.matrix[i])
		atomic.AddUint64(&cms.matrix[i], otherVal)
	}

	atomic.AddUint64(&cms.count, atomic.LoadUint64(&other.count))

	return nil
}

func getCMSHashes(data []byte) (h1, h2 uint32) {
	var h uint64 = fnvOffset64
	for _, b := range data {
		h ^= uint64(b)
		h *= fnvPrime64
	}

	return uint32(h), uint32(h >> 32)
}
