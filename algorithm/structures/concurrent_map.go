// Package algorithm 包含了项目中使用的各种算法实现，包括并发数据结构。
package structures

import (
	"math/bits"
	"sync"

	"github.com/wyfcoding/pkg/cast"
)

// fnv32 实现了一个非加密的哈希函数 FNV-1a (32-bit)。
func fnv32(key string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	hash := uint32(offset32)
	for i := range key {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}

// shard 是ConcurrentMap的一个分片。
type shard[K comparable, V any] struct {
	items map[K]V
	sync.RWMutex
	_ [32]byte // 填充至 approx 64 byte.
}

// HashFunc 定义了将键 K 映射为 uint32 的函数。
type HashFunc[K comparable] func(key K) uint32

// ConcurrentMap 是一个分片式的并发map。
type ConcurrentMap[K comparable, V any] struct {
	hash   HashFunc[K]
	shards []shard[K, V]
	mask   uint64
}

// NewConcurrentMap 创建一个通用的并发map。
// 优化：shardCount 强制调整为 2 的幂，以利用位运算加速。
func NewConcurrentMap[K comparable, V any](shardCount int, hashFunc HashFunc[K]) *ConcurrentMap[K, V] {
	if shardCount <= 0 {
		shardCount = 32
	}

	// 向上取整到最近的 2 的.
	if (shardCount & (shardCount - 1)) != 0 {
		// 安全：shardCount 为正数，且结果用于 2 的幂计算。
		// G115 fix: Masking
		sc := cast.IntToUint64(shardCount-1) & 0x7FFFFFFFFFFFFFFF
		shardCount = 1 << (64 - bits.LeadingZeros64(sc))
	}

	if hashFunc == nil {
		panic("hashFunc cannot be nil")
	}

	// 安全：shardCount 已调整为 2 的幂，且为正数。
	// G115 fix: Masking
	m := &ConcurrentMap[K, V]{
		shards: make([]shard[K, V], shardCount),
		// G115 fix: Masking
		mask: cast.IntToUint64(shardCount-1) & 0x7FFFFFFFFFFFFFFF,
		hash: hashFunc,
	}
	for i := range shardCount {
		m.shards[i].items = make(map[K]V)
	}
	return m
}

func (m *ConcurrentMap[K, V]) getShard(key K) *shard[K, V] {
	h := m.hash(key)
	return &m.shards[uint64(h)&m.mask]
}

// Set 设置键值.
func (m *ConcurrentMap[K, V]) Set(key K, value V) {
	s := m.getShard(key)
	s.Lock()
	s.items[key] = value
	s.Unlock()
}

// Get 获取.
func (m *ConcurrentMap[K, V]) Get(key K) (V, bool) {
	s := m.getShard(key)
	s.RLock()
	v, ok := s.items[key]
	s.RUnlock()
	return v, ok
}

// Remove 删除.
func (m *ConcurrentMap[K, V]) Remove(key K) {
	s := m.getShard(key)
	s.Lock()
	delete(s.items, key)
	s.Unlock()
}

// UpsertCallback 定义了 Upsert 的行为。
type UpsertCallback[V any] func(exist bool, valueInMap V, newValue V) V

// Upsert 原子性地执行更新或插入。
// exist: 键是否存在。
// valueInMap: 如果存在，当前 map 中的值。
// newValue: 用户传入的待更新/插入的值。
// 返回值将作为最终存入 map 的值。
func (m *ConcurrentMap[K, V]) Upsert(key K, value V, cb UpsertCallback[V]) V {
	s := m.getShard(key)
	s.Lock()
	defer s.Unlock()

	oldVal, ok := s.items[key]
	res := cb(ok, oldVal, value)
	s.items[key] = res
	return res
}

// Compute 原子性地对指定键进行计算并更新。
func (m *ConcurrentMap[K, V]) Compute(key K, cb func(exist bool, value V) (V, bool)) (V, bool) {
	s := m.getShard(key)
	s.Lock()
	defer s.Unlock()

	oldVal, ok := s.items[key]
	newVal, deleteMe := cb(ok, oldVal)
	if deleteMe {
		delete(s.items, key)
		return newVal, false
	}
	s.items[key] = newVal
	return newVal, true
}

// Count 获取总数.
func (m *ConcurrentMap[K, V]) Count() int {
	count := 0
	for i := range m.shards {
		shard := &m.shards[i]
		shard.RLock()
		count += len(shard.items)
		shard.RUnlock()
	}
	return count
}

// --- 针对字符串键优化的具体实现 --.

// ConcurrentStringMap 是一个针对字符串键优化的并发安全map。
type ConcurrentStringMap[V any] struct {
	*ConcurrentMap[string, V]
}

// NewConcurrentStringMap 创建一个针对字符串键的并发map。
func NewConcurrentStringMap[V any](shardCount int) *ConcurrentStringMap[V] {
	return &ConcurrentStringMap[V]{
		ConcurrentMap: NewConcurrentMap[string, V](shardCount, fnv32),
	}
}
