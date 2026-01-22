package structures

import (
	"sync"
)

// HashFunc 定义了泛型哈希函数原型。
type HashFunc[K comparable] func(key K) uint32

// ConcurrentMap 线程安全的分片 Map 实现。
type ConcurrentMap[K comparable, V any] struct {
	hashFunc HashFunc[K]
	shards   []*shard[K, V]
	count    uint32
}

type shard[K comparable, V any] struct {
	items map[K]V
	mu    sync.RWMutex
}

// NewConcurrentMap 创建一个线程安全的分片 Map。
func NewConcurrentMap[K comparable, V any](shardCount uint32, hashFunc HashFunc[K]) *ConcurrentMap[K, V] {
	if shardCount == 0 {
		shardCount = 16
	}
	m := &ConcurrentMap[K, V]{
		shards:   make([]*shard[K, V], shardCount),
		hashFunc: hashFunc,
		count:    shardCount,
	}
	for i := range shardCount {
		m.shards[i] = &shard[K, V]{items: make(map[K]V)}
	}
	return m
}

func (m *ConcurrentMap[K, V]) getShard(key K) *shard[K, V] {
	return m.shards[m.hashFunc(key)%m.count]
}

// Set 设置键值对。
func (m *ConcurrentMap[K, V]) Set(key K, value V) {
	shard := m.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	shard.items[key] = value
}

// Get 获取值。
func (m *ConcurrentMap[K, V]) Get(key K) (V, bool) {
	shard := m.getShard(key)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	val, ok := shard.items[key]
	return val, ok
}

// Delete 删除键。
func (m *ConcurrentMap[K, V]) Delete(key K) {
	shard := m.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	delete(shard.items, key)
}

// UpsertCallback 存在即更新的回调函数。
type UpsertCallback[V any] func(exist bool, valueInMap V, newValue V) V

// Upsert 原子地执行创建或更新。
func (m *ConcurrentMap[K, V]) Upsert(key K, value V, cb UpsertCallback[V]) V {
	shard := m.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	old, ok := shard.items[key]
	res := cb(ok, old, value)
	shard.items[key] = res
	return res
}

// ConcurrentStringMap 专为 string 类型优化的分片 Map。
type ConcurrentStringMap[V any] struct {
	*ConcurrentMap[string, V]
}

// NewConcurrentStringMap 创建 string 类型的分片 Map。
func NewConcurrentStringMap[V any](shardCount uint32) *ConcurrentStringMap[V] {
	// 简单的 string 哈希函数
	hash := func(key string) uint32 {
		var h uint32
		for i := range len(key) {
			h = 31*h + uint32(key[i])
		}
		return h
	}
	return &ConcurrentStringMap[V]{
		ConcurrentMap: NewConcurrentMap[string, V](shardCount, hash),
	}
}
