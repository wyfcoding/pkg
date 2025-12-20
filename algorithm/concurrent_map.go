// Package algorithm 包含了项目中使用的各种算法实现，包括并发数据结构。
package algorithm

import (
	"sync"
)

// shard 是ConcurrentMap的一个分片。
// 每个分片包含一个独立的读写锁和一个map，用于存储一部分键值对。
type shard[K comparable, V any] struct {
	sync.RWMutex
	items map[K]V
}

// --- Generic ConcurrentMap (Incomplete) ---

// ConcurrentMap 是一个分片式的并发map的通用结构模板。
// 通过将键值对分散到不同的分片（shard）中，可以显著减少多goroutine并发访问时的锁竞争。
// 注意：这个泛型版本缺少一个关键的 `getShard` 方法，因为它需要一个通用的哈希函数来处理泛型键 `K`。
// 因此，这个结构本身并未完全实现，下面的 `ConcurrentStringMap` 是一个针对string键的具体实现。
type ConcurrentMap[K comparable, V any] struct {
	shards     []*shard[K, V]
	shardCount uint64
}

// NewConcurrentMap 创建一个通用的并发map。
// shardCount 指定了分片的数量，该值越大，理论上锁竞争越小，但内存开销也越大。
// 如果shardCount小于等于0，则使用默认值32。
func NewConcurrentMap[K comparable, V any](shardCount int) *ConcurrentMap[K, V] {
	if shardCount <= 0 {
		shardCount = 32 // 默认分片数
	}
	m := &ConcurrentMap[K, V]{
		shards:     make([]*shard[K, V], shardCount),
		shardCount: uint64(shardCount),
	}
	for i := 0; i < shardCount; i++ {
		m.shards[i] = &shard[K, V]{items: make(map[K]V)}
	}
	return m
}

// --- 针对字符串键优化的并发 Map ---

// ConcurrentStringMap 是一个针对字符串键优化的并发安全map。
// 它内部使用了分片技术来提高并发性能。
type ConcurrentStringMap[V any] struct {
	shards     []*shard[string, V]
	shardCount uint64
}

// NewConcurrentStringMap 创建一个针对字符串键的并发map。
func NewConcurrentStringMap[V any](shardCount int) *ConcurrentStringMap[V] {
	if shardCount <= 0 {
		shardCount = 32
	}
	m := &ConcurrentStringMap[V]{
		shards:     make([]*shard[string, V], shardCount),
		shardCount: uint64(shardCount),
	}
	for i := 0; i < shardCount; i++ {
		m.shards[i] = &shard[string, V]{items: make(map[string]V)}
	}
	return m
}

// getShard 根据给定的键，通过哈希计算决定其应属于哪个分片。
func (m *ConcurrentStringMap[V]) getShard(key string) *shard[string, V] {
	hash := fnv32(key)
	// 使用取模运算将哈希值映射到分片索引
	return m.shards[uint64(hash)%m.shardCount]
}

// fnv32 实现了一个非加密的哈希函数 FNV-1a (32-bit)。
// 它用于将字符串键均匀地映射到不同的分片。
func fnv32(key string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	hash := uint32(offset32)
	for i := 0; i < len(key); i++ {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}

// Set 向map中设置一个键值对。
// 这个操作是并发安全的。
func (m *ConcurrentStringMap[V]) Set(key string, value V) {
	// 1. 找到键对应的分片
	shard := m.getShard(key)
	// 2. 锁定该分片（写锁）
	shard.Lock()
	// 3. 在分片的map中设置值
	shard.items[key] = value
	// 4. 解锁
	shard.Unlock()
}

// Get 从map中获取一个键对应的值。
// 如果键存在，返回其值和 true；否则，返回零值和 false。
// 这个操作是并发安全的。
func (m *ConcurrentStringMap[V]) Get(key string) (V, bool) {
	// 1. 找到键对应的分片
	shard := m.getShard(key)
	// 2. 锁定该分片（读锁）
	shard.RLock()
	// 3. 从分片的map中读取值
	val, ok := shard.items[key]
	// 4. 解锁
	shard.RUnlock()
	return val, ok
}

// Remove 从map中删除一个键值对。
// 这个操作是并发安全的。
func (m *ConcurrentStringMap[V]) Remove(key string) {
	// 1. 找到键对应的分片
	shard := m.getShard(key)
	// 2. 锁定该分片（写锁）
	shard.Lock()
	// 3. 从分片的map中删除键
	delete(shard.items, key)
	// 4. 解锁
	shard.Unlock()
}
