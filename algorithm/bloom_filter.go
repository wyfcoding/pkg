package algorithm

import (
	"hash/fnv"
	"math"
	"sync"
)

// BloomFilter 工业级布隆过滤器
type BloomFilter struct {
	mu    sync.RWMutex
	bits  []uint64
	size  uint
	hashes uint
}

// NewBloomFilter 创建一个新的布隆过滤器
// n: 预估存放的元素数量
// p: 允许的误报率 (如 0.01 代表 1%)
func NewBloomFilter(n uint, p float64) *BloomFilter {
	// 计算最优的位数组大小 m
	m := uint(math.Ceil(-float64(n) * math.Log(p) / math.Pow(math.Log(2), 2)))
	// 计算最优的哈希函数个数 k
	k := uint(math.Ceil(float64(m) / float64(n) * math.Log(2)))

	return &BloomFilter{
		bits:   make([]uint64, (m+63)/64),
		size:   m,
		hashes: k,
	}
}

// Add 向过滤器添加元素
func (bf *BloomFilter) Add(data []byte) {
	bf.mu.Lock()
	defer bf.mu.Unlock()

	h1, h2 := bf.hash(data)
	for i := uint(0); i < bf.hashes; i++ {
		// 使用双重哈希法模拟多个哈希函数: gi(x) = h1(x) + i*h2(x)
		idx := (h1 + i*h2) % bf.size
		bf.bits[idx/64] |= (1 << (idx % 64))
	}
}

// Contains 检查元素是否可能存在
func (bf *BloomFilter) Contains(data []byte) bool {
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	h1, h2 := bf.hash(data)
	for i := uint(0); i < bf.hashes; i++ {
		idx := (h1 + i*h2) % bf.size
		if bf.bits[idx/64]&(1<<(idx%64)) == 0 {
			return false // 只要有一位是0，元素一定不存在
		}
	}
	return true // 可能存在
}

// hash 使用 FNV-1a 生成两个 64 位哈希值
func (bf *BloomFilter) hash(data []byte) (uint, uint) {
	h := fnv.New64a()
	h.Write(data)
	v := h.Sum64()
	return uint(v >> 32), uint(v)
}
