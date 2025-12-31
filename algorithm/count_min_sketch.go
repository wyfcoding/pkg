package algorithm

import (
	"errors"
	"hash/fnv"
	"math"
	"math/rand"
	"sync"
	"time"
)

// CountMinSketch 是一种概率数据结构，用于频率估计（Frequency Estimation）。
// 它使用固定大小的内存来统计数据流中元素的出现次数。
// 适用于：风控（IP访问频率）、热点数据发现（TopK）、缓存淘汰策略等。
// 特点：
// 1. 空间复杂度固定，不随数据量增加。
// 2. 支持并发写入（通过分段锁或原子操作，本实现使用互斥锁以保证通用性，高性能场景可改为原子操作）。
// 3. 存在误差：估计值 >= 真实值（Overestimation），但不会低估。
type CountMinSketch struct {
	width  uint   // 矩阵宽度 (w)
	depth  uint   // 矩阵深度 (d)，即哈希函数的数量
	count  uint64 // 总添加次数
	matrix [][]uint64
	seeds  []uint32     // 哈希种子
	mu     sync.RWMutex // 并发安全锁
}

// NewCountMinSketch 创建一个新的 CountMinSketch
// epsilon: 允许的误差率 (例如 0.01)
// delta: 估计失败的概率 (例如 0.01)
// 宽度 w = ceil(e / epsilon)
// 深度 d = ceil(ln(1 / delta))
func NewCountMinSketch(epsilon, delta float64) (*CountMinSketch, error) {
	if epsilon <= 0 || epsilon >= 1 {
		return nil, errors.New("epsilon must be between 0 and 1")
	}
	if delta <= 0 || delta >= 1 {
		return nil, errors.New("delta must be between 0 and 1")
	}

	width := uint(math.Ceil(math.E / epsilon))
	depth := uint(math.Ceil(math.Log(1 / delta)))

	matrix := make([][]uint64, depth)
	for i := range matrix {
		matrix[i] = make([]uint64, width)
	}

	seeds := make([]uint32, depth)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range seeds {
		seeds[i] = r.Uint32()
	}

	return &CountMinSketch{
		width:  width,
		depth:  depth,
		matrix: matrix,
		seeds:  seeds,
	}, nil
}

// Add 增加元素的计数
func (cms *CountMinSketch) Add(data []byte, count uint64) {
	cms.mu.Lock()
	defer cms.mu.Unlock()

	cms.count += count
	// 计算 d 个哈希值
	h1, h2 := hash(data)

	for i := uint(0); i < cms.depth; i++ {
		// 使用 Double Hashing 技术生成 d 个哈希值，避免计算 d 次哈希
		// index = (h1 + i*h2 + seed) % width
		index := (uint(h1) + uint(h2)*i + uint(cms.seeds[i])) % cms.width
		cms.matrix[i][index] += count
	}
}

// AddString 增加字符串元素的计数
func (cms *CountMinSketch) AddString(key string, count uint64) {
	cms.Add([]byte(key), count)
}

// Estimate 估算元素的出现频率
func (cms *CountMinSketch) Estimate(data []byte) uint64 {
	cms.mu.RLock()
	defer cms.mu.RUnlock()

	minCount := uint64(math.MaxUint64)
	h1, h2 := hash(data)

	for i := uint(0); i < cms.depth; i++ {
		index := (uint(h1) + uint(h2)*i + uint(cms.seeds[i])) % cms.width
		count := cms.matrix[i][index]
		if count < minCount {
			minCount = count
		}
	}

	return minCount
}

// EstimateString 估算字符串元素的出现频率
func (cms *CountMinSketch) EstimateString(key string) uint64 {
	return cms.Estimate([]byte(key))
}

// Reset 重置所有计数
func (cms *CountMinSketch) Reset() {
	cms.mu.Lock()
	defer cms.mu.Unlock()

	for i := range cms.matrix {
		for j := range cms.matrix[i] {
			cms.matrix[i][j] = 0
		}
	}
	cms.count = 0
}

// TotalCount 返回总的添加次数
func (cms *CountMinSketch) TotalCount() uint64 {
	cms.mu.RLock()
	defer cms.mu.RUnlock()
	return cms.count
}

// hash 使用 FNV-1a 算法生成两个基础哈希值，用于 Double Hashing
func hash(data []byte) (uint32, uint32) {
	h := fnv.New64a()
	h.Write(data)
	sum := h.Sum64()
	// 将 64 位哈希拆分为两个 32 位哈希
	return uint32(sum), uint32(sum >> 32)
}

// Merge 将另一个 CMS 合并到当前 CMS 中 (必须具有相同的维度和种子)
func (cms *CountMinSketch) Merge(other *CountMinSketch) error {
	cms.mu.Lock()
	defer cms.mu.Unlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	if cms.width != other.width || cms.depth != other.depth {
		return errors.New("cannot merge CountMinSketch with different dimensions")
	}

	// 简单的种子检查
	if len(cms.seeds) != len(other.seeds) || cms.seeds[0] != other.seeds[0] {
		return errors.New("cannot merge CountMinSketch with different seeds")
	}

	for i := range cms.matrix {
		for j := range cms.matrix[i] {
			cms.matrix[i][j] += other.matrix[i][j]
		}
	}
	cms.count += other.count
	return nil
}
