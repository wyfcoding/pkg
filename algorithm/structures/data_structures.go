// Package structures 提供通用的高性能内存数据结构.
package structures

import (
	"sync"
	"time"
)

// ConcurrentCache 并发安全的泛型缓存，支持延迟过期清理逻辑。
type ConcurrentCache[V any] struct {
	data     map[string]V
	expireAt map[string]int64 // 过期时间戳 (Unix Seconds)
	mu       sync.RWMutex
}

// NewConcurrentCache 创建一个新的并发安全缓存实例。
func NewConcurrentCache[V any]() *ConcurrentCache[V] {
	return &ConcurrentCache[V]{
		data:     make(map[string]V),
		expireAt: make(map[string]int64),
	}
}

// Set 存储一个键值对，并设置过期时间。
func (cc *ConcurrentCache[V]) Set(key string, value V, expireAt int64) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.data[key] = value
	cc.expireAt[key] = expireAt
}

// Get 根据键获取值，如果已过期或不存在则返回零值。
func (cc *ConcurrentCache[V]) Get(key string) (V, bool) {
	cc.mu.RLock()
	expiry, hasExpiry := cc.expireAt[key]
	val, ok := cc.data[key]
	cc.mu.RUnlock()

	if !ok {
		var zero V
		return zero, false
	}

	// 检查过期 (Lazy Expiration)
	if hasExpiry && expiry > 0 && time.Now().Unix() > expiry {
		cc.mu.Lock()
		// 双重检查
		if exp, exists := cc.expireAt[key]; exists && exp > 0 && time.Now().Unix() > exp {
			delete(cc.data, key)
			delete(cc.expireAt, key)
		}
		cc.mu.Unlock()
		var zero V
		return zero, false
	}

	return val, true
}

// Delete 从缓存中移除指定的键。
func (cc *ConcurrentCache[V]) Delete(key string) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	delete(cc.data, key)
	delete(cc.expireAt, key)
}

// Clear 清空所有缓存数据。
func (cc *ConcurrentCache[V]) Clear() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.data = make(map[string]V)
	cc.expireAt = make(map[string]int64)
}