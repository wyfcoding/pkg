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
	"errors"
	"log/slog"
	"math"
	"sync"
)

// BloomFilter 封装了高空间利用率的概率型数据结构。
type BloomFilter struct {
	mu     sync.RWMutex
	bits   []uint64 // 底层位数组。
	size   uint     // 位数组总长度 (m)。
	hashes uint     // 最优哈希函数个数 (k)。
}

// NewBloomFilter 根据预估容量与允许的误报率，科学计算并初始化布隆过滤器。
func NewBloomFilter(n uint, p float64) (*BloomFilter, error) {
	if n == 0 {
		return nil, errors.New("n (expected elements) must be greater than 0")
	}
	if p <= 0 || p >= 1 {
		return nil, errors.New("p (false positive rate) must be in range (0, 1)")
	}

	m := uint(math.Ceil(-float64(n) * math.Log(p) / math.Pow(math.Log(2), 2)))
	k := uint(math.Ceil(float64(m) / float64(n) * math.Log(2)))

	slog.Info("bloom_filter initialized", "expected_elements", n, "false_positive_rate", p, "bits_size", m, "hashes_count", k)

	return &BloomFilter{
		bits:   make([]uint64, (m+63)/64),
		size:   m,
		hashes: k,
	}, nil
}

// Add 向布隆过滤器中插入新的数据。
func (bf *BloomFilter) Add(data []byte) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	h1, h2 := bf.hash(data)
	for i := uint(0); i < bf.hashes; i++ {
		// 使用 h1 + i*h2 模拟多次哈希 (Kirsch-Mitzenmacher optimization)。
		// 注意：这里可能会溢出，利用 uint 的溢出特性是安全的。
		idx := (h1 + i*h2) % bf.size
		// idx / 64 -> idx >> 6。
		// idx % 64 -> idx & 63。
		bf.bits[idx>>6] |= (1 << (idx & 63))
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
		if bf.bits[idx>>6]&(1<<(idx&63)) == 0 {
			return false
		}
	}
	return true
}

// hash 使用 FNV-1a 生成两个 64 位哈希值 (Zero Allocation)。
func (bf *BloomFilter) hash(data []byte) (uint, uint) {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for _, b := range data {
		h ^= uint64(b)
		h *= prime64
	}
	return uint(h >> 32), uint(h)
}
