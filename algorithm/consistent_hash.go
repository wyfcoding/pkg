package algorithm

import (
	"fmt"
	"hash/crc32"
	"sort"
	"sync"
)

// Hash 函数定义
type Hash func(data []byte) uint32

// ConsistentHash 一致性哈希算法实现
type ConsistentHash struct {
	hash     Hash
	replicas int            // 虚拟节点倍数
	keys     []int          // 已排序的哈希环
	hashMap  map[int]string // 虚拟节点哈希值到物理节点名称的映射
	mu       sync.RWMutex
}

// NewConsistentHash 创建一致性哈希实例
// replicas: 每个物理节点对应的虚拟节点数量，用于解决数据倾斜问题
// fn: 自定义哈希函数，若为 nil 则默认使用 CRC32
func NewConsistentHash(replicas int, fn Hash) *ConsistentHash {
	ch := &ConsistentHash{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[int]string),
	}
	if ch.hash == nil {
		ch.hash = crc32.ChecksumIEEE
	}
	return ch
}

// Add 添加物理节点到哈希环
func (ch *ConsistentHash) Add(nodes ...string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	for _, node := range nodes {
		for i := 0; i < ch.replicas; i++ {
			// 为每个虚拟节点计算哈希值
			hash := int(ch.hash([]byte(fmt.Sprintf("%d%s", i, node))))
			ch.keys = append(ch.keys, hash)
			ch.hashMap[hash] = node
		}
	}
	sort.Ints(ch.keys) // 保持哈希环有序
}

// Get 根据 Key 获取最近的物理节点
func (ch *ConsistentHash) Get(key string) string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.keys) == 0 {
		return ""
	}

	hash := int(ch.hash([]byte(key)))

	// 通过二分查找在环上寻找第一个大于等于该哈希值的虚拟节点
	idx := sort.Search(len(ch.keys), func(i int) bool {
		return ch.keys[i] >= hash
	})

	// 若到了环的末尾，则返回环的第一个节点（体现“环”的特性）
	if idx == len(ch.keys) {
		idx = 0
	}

	return ch.hashMap[ch.keys[idx]]
}

// Remove 从哈希环中移除物理节点
func (ch *ConsistentHash) Remove(node string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	for i := 0; i < ch.replicas; i++ {
		hash := int(ch.hash([]byte(fmt.Sprintf("%d%s", i, node))))
		// 从有序 keys 中移除
		idx := sort.SearchInts(ch.keys, hash)
		if idx < len(ch.keys) && ch.keys[idx] == hash {
			ch.keys = append(ch.keys[:idx], ch.keys[idx+1:]...)
		}
		delete(ch.hashMap, hash)
	}
}
