package algorithm

import (
	"errors"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync/atomic"
	"time"
)

// CountMinSketch 高性能概率数据结构
type CountMinSketch struct {
	width  uint
	depth  uint
	count  uint64   // 总计数，使用 atomic
	matrix []uint64 // 扁平化矩阵，便于原子操作 (index = depth_idx * width + width_idx)
	seeds  []uint32
}

// NewCountMinSketch 创建 CMS。
func NewCountMinSketch(epsilon, delta float64) (*CountMinSketch, error) {
	if epsilon <= 0 || epsilon >= 1 || delta <= 0 || delta >= 1 {
		return nil, errors.New("invalid epsilon or delta")
	}

	width := uint(math.Ceil(math.E / epsilon))
	depth := uint(math.Ceil(math.Log(1 / delta)))

	slog.Info("CountMinSketch initialized", "epsilon", epsilon, "delta", delta, "width", width, "depth", depth)

	return &CountMinSketch{
		width:  width,
		depth:  depth,
		matrix: make([]uint64, depth*width),
		seeds:  generateSeeds(depth),
	}, nil
}

func generateSeeds(depth uint) []uint32 {
	seeds := make([]uint32, depth)
	for i := range seeds {
		seeds[i] = rand.Uint32()
	}
	return seeds
}

// Add 原子增加元素的计数
func (cms *CountMinSketch) Add(data []byte, count uint64) {
	atomic.AddUint64(&cms.count, count)
	h1, h2 := hash(data)

	for i := uint(0); i < cms.depth; i++ {
		index := (uint(h1) + uint(h2)*i + uint(cms.seeds[i])) % cms.width
		atomic.AddUint64(&cms.matrix[i*cms.width+index], count)
	}
}

// AddString 增加字符串元素的计数
func (cms *CountMinSketch) AddString(key string, count uint64) {
	cms.Add([]byte(key), count)
}

// Estimate 估算频率
func (cms *CountMinSketch) Estimate(data []byte) uint64 {
	minCount := uint64(math.MaxUint64)
	h1, h2 := hash(data)

	for i := uint(0); i < cms.depth; i++ {
		index := (uint(h1) + uint(h2)*i + uint(cms.seeds[i])) % cms.width
		count := atomic.LoadUint64(&cms.matrix[i*cms.width+index])
		if count < minCount {
			minCount = count
		}
	}
	return minCount
}

// EstimateString 估算字符串频率
func (cms *CountMinSketch) EstimateString(key string) uint64 {
	return cms.Estimate([]byte(key))
}

// Decay 衰减机制：将所有计数值减半
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

// Reset 重置所有计数
func (cms *CountMinSketch) Reset() {
	for i := range cms.matrix {
		atomic.StoreUint64(&cms.matrix[i], 0)
	}
	atomic.StoreUint64(&cms.count, 0)
}

// TotalCount 返回总的添加次数
func (cms *CountMinSketch) TotalCount() uint64 {
	return atomic.LoadUint64(&cms.count)
}

// Merge 合并另一个 CMS
func (cms *CountMinSketch) Merge(other *CountMinSketch) error {
	if cms.width != other.width || cms.depth != other.depth {
		return errors.New("cannot merge CountMinSketch with different dimensions")
	}

	for i := range cms.matrix {
		otherVal := atomic.LoadUint64(&other.matrix[i])
		atomic.AddUint64(&cms.matrix[i], otherVal)
	}
	atomic.AddUint64(&cms.count, atomic.LoadUint64(&other.count))
	return nil
}

// hash 使用 FNV-1a 算法生成两个基础哈希值 (Zero Allocation)
func hash(data []byte) (uint32, uint32) {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for _, b := range data {
		h ^= uint64(b)
		h *= prime64
	}
	return uint32(h), uint32(h >> 32)
}
