// Package algorithm 提供了高性能算法集合.
package infra

import (
	"hash/crc32"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"unsafe"
)

const (
	defaultBufSize = 64
	baseTen        = 10
)

// Hash 定义哈希计算函数的类型原型.
type Hash func(data []byte) uint32

// ConsistentHash 维护了一致性哈希环的状态.
type ConsistentHash struct {
	hashMap  map[int]string
	hash     Hash
	keys     []int
	mu       sync.RWMutex
	replicas int
}

// NewConsistentHash 初始化并返回一个新的哈希环实例.
func NewConsistentHash(replicas int, hashFn Hash) *ConsistentHash {
	ring := &ConsistentHash{
		replicas: replicas,
		hash:     hashFn,
		hashMap:  make(map[int]string),
		keys:     nil,
		mu:       sync.RWMutex{},
	}

	if ring.hash == nil {
		ring.hash = crc32.ChecksumIEEE
	}

	slog.Info("consistent_hash initialized", "replicas", replicas)

	return ring
}

// Add 将一个或多个物理节点加入哈希环.
func (ch *ConsistentHash) Add(nodes ...string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	buf := make([]byte, 0, defaultBufSize)

	for _, node := range nodes {
		for i := range ch.replicas {
			buf = buf[:0]
			buf = strconv.AppendInt(buf, int64(i), baseTen)
			buf = append(buf, node...)

			hashVal := int(ch.hash(buf))
			ch.keys = append(ch.keys, hashVal)
			ch.hashMap[hashVal] = node
		}

		slog.Info("node added to consistent_hash ring", "node", node, "virtual_nodes", ch.replicas)
	}

	slices.Sort(ch.keys)
}

// Get 根据输入的键名查找其在哈希环上顺时针方向最近的物理节点.
func (ch *ConsistentHash) Get(key string) string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.keys) == 0 {
		return ""
	}

	buf := unsafe.Slice(unsafe.StringData(key), len(key))
	hashVal := int(ch.hash(buf))

	idx, _ := slices.BinarySearch(ch.keys, hashVal)

	if idx == len(ch.keys) {
		idx = 0
	}

	return ch.hashMap[ch.keys[idx]]
}

// Remove 将指定的物理节点及其所有虚拟节点从哈希环中安全移除.
func (ch *ConsistentHash) Remove(node string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	toRemove := make(map[int]struct{}, ch.replicas)
	buf := make([]byte, 0, defaultBufSize)

	for i := range ch.replicas {
		buf = buf[:0]
		buf = strconv.AppendInt(buf, int64(i), baseTen)
		buf = append(buf, node...)

		hashVal := int(ch.hash(buf))
		toRemove[hashVal] = struct{}{}
		delete(ch.hashMap, hashVal)
	}

	newKeys := make([]int, 0, len(ch.keys)-len(toRemove))
	for _, k := range ch.keys {
		if _, ok := toRemove[k]; !ok {
			newKeys = append(newKeys, k)
		}
	}

	ch.keys = newKeys

	slog.Info("node removed from consistent_hash ring", "node", node)
}
