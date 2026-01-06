// Package algorithm 提供了高性能/ACM/竞赛算法集合。
// 此文件实现了工业级布隆过滤器 (Bloom Filter)，适用于海量数据去重与售罄快速预检。
//
// 性能优化点：
// 1. 空间效率：基于位数组（Bitset）实现，内存占用极低。
// 2. 哈希优化：采用 FNV-1a 双重哈希法模拟 K 个哈希函数，降低计算开销。
// 3. 并发安全：使用读写锁（RWMutex）保护状态。
//
// 复杂度分析：
// - 添加元素 (Add): O(K)，K 为哈希函数个数。
// - 成员检查 (Contains): O(K)。
// - 空间复杂度: O(M)，M 为计算出的位数组长度。
package algorithm

import (
	"hash/fnv"
	"log/slog"
	"math"
	"sync"
)

// BloomFilter 封装了高空间利用率的概率型数据结构。
type BloomFilter struct {
	mu     sync.RWMutex
	bits   []uint64 // 底层位数组
	size   uint     // 位数组总长度 (m)
	hashes uint     // 最优哈希函数个数 (k)
}

// NewBloomFilter 根据预估容量与允许的误报率，科学计算并初始化布隆过滤器。
func NewBloomFilter(n uint, p float64) *BloomFilter {
	m := uint(math.Ceil(-float64(n) * math.Log(p) / math.Pow(math.Log(2), 2)))
	k := uint(math.Ceil(float64(m) / float64(n) * math.Log(2)))

	slog.Info("bloom_filter initialized", "expected_elements", n, "false_positive_rate", p, "bits_size", m, "hashes_count", k)

	return &BloomFilter{
		bits:   make([]uint64, (m+63)/64),
		size:   m,
		hashes: k,
	}
}

// Add 向布隆过滤器中插入新的数据。
func (bf *BloomFilter) Add(data []byte) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	h1, h2 := bf.hash(data)
	for i := uint(0); i < bf.hashes; i++ {
		idx := (h1 + i*h2) % bf.size
		bf.bits[idx/64] |= (1 << (idx % 64))
	}
}

// Contains 执行成员存在性检查。
// 注意：返回 true 表示元素“可能”存在（受误报率影响），返回 false 表示元素“一定”不存在。
func (bf *BloomFilter) Contains(data []byte) bool {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	h1, h2 := bf.hash(data)
	for i := uint(0); i < bf.hashes; i++ {
		idx := (h1 + i*h2) % bf.size
		if bf.bits[idx/64]&(1<<(idx%64)) == 0 {
			return false
		}
	}
	return true
}

// hash 使用 FNV-1a 生成两个 64 位哈希值
func (bf *BloomFilter) hash(data []byte) (uint, uint) {
	h := fnv.New64a()
	h.Write(data)
	v := h.Sum64()
	return uint(v >> 32), uint(v)
}
