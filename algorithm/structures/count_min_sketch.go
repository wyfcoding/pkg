package structures

import (
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	algomath "github.com/wyfcoding/pkg/algorithm/math"
	"github.com/wyfcoding/pkg/xerrors"
)

// CountMinSketch 高性能概率数据结构泛型实现.
type CountMinSketch[T Hashable] struct {
	matrix []uint64
	seeds  []uint32
	width  uint
	depth  uint
	count  uint64
}

// NewCountMinSketch 创建 CMS.
func NewCountMinSketch[T Hashable](epsilon, delta float64) (*CountMinSketch[T], error) {
	if epsilon <= 0 || epsilon >= 1 || delta <= 0 || delta >= 1 {
		return nil, xerrors.ErrInvalidEpsilonDelta
	}

	width := uint(math.Ceil(math.E / epsilon))
	depth := uint(math.Ceil(math.Log(1 / delta)))

	slog.Info("CountMinSketch initialized", "epsilon", epsilon, "delta", delta, "width", width, "depth", depth)

	return &CountMinSketch[T]{
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
		var b [4]byte
		_, _ = rand.Read(b[:])
		seeds[i] = binary.LittleEndian.Uint32(b[:])
	}
	return seeds
}

// Add 原子增加元素的计数.
func (cms *CountMinSketch[T]) Add(item T, count uint64) {
	data, err := item.MarshalBinary()
	if err != nil {
		return
	}
	atomic.AddUint64(&cms.count, count)
	h1, h2 := getCMSHashes(data)

	for i := range cms.depth {
		index := (uint(h1) + uint(h2)*i + uint(cms.seeds[i])) % cms.width
		atomic.AddUint64(&cms.matrix[i*cms.width+index], count)
	}
}

// Estimate 估算频率.
func (cms *CountMinSketch[T]) Estimate(item T) uint64 {
	data, err := item.MarshalBinary()
	if err != nil {
		return 0
	}
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

// Decay 衰减机制：将所有计数值减半.
func (cms *CountMinSketch[T]) Decay() {
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
func (cms *CountMinSketch[T]) Reset() {
	for i := range cms.matrix {
		atomic.StoreUint64(&cms.matrix[i], 0)
	}
	atomic.StoreUint64(&cms.count, 0)
}

// TotalCount 返回总的添加次数.
func (cms *CountMinSketch[T]) TotalCount() uint64 {
	return atomic.LoadUint64(&cms.count)
}

// Merge 合并另一个 CMS.
func (cms *CountMinSketch[T]) Merge(other *CountMinSketch[T]) error {
	if cms.width != other.width || cms.depth != other.depth {
		return xerrors.ErrDimMismatch
	}

	for i := range cms.matrix {
		otherVal := atomic.LoadUint64(&other.matrix[i])
		atomic.AddUint64(&cms.matrix[i], otherVal)
	}

	atomic.AddUint64(&cms.count, atomic.LoadUint64(&other.count))
	return nil
}

func getCMSHashes(data []byte) (h1, h2 uint32) {
	var h uint64 = algomath.FnvOffset64
	for _, b := range data {
		h ^= uint64(b)
		h *= algomath.FnvPrime64
	}

	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], h)
	h1 = binary.LittleEndian.Uint32(b[:4])
	h2 = binary.LittleEndian.Uint32(b[4:])
	return
}
