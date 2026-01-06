// Package algorithm 提供了高性能算法集合。
// 此文件实现了一致性哈希 (Consistent Hashing) 算法，包含虚拟节点支持，常用于分布式缓存路由与负载均衡。
//
// 复杂度分析：
// - 添加节点 (Add): O(K * log(N*K))，K 为虚拟节点倍数，N 为当前物理节点数。
// - 获取节点 (Get): O(log(N*K))，采用二分查找加速寻址。
// - 移除节点 (Remove): O(K * log(N*K))。
package algorithm

import (
	"hash/crc32"
	"log/slog"
	"sort"
	"strconv"
	"sync"
)

// Hash 定义哈希计算函数的类型原型。
type Hash func(data []byte) uint32

// ConsistentHash 维护了一致性哈希环的状态。
type ConsistentHash struct {
	hash     Hash           // 哈希计算函数 (默认 CRC32)
	replicas int            // 每个物理节点映射的虚拟节点倍数，用于平滑负载
	keys     []int          // 排序后的哈希环，存储所有虚拟节点的哈希值
	hashMap  map[int]string // 虚拟节点哈希值到物理节点标识的快速映射
	mu       sync.RWMutex
}

// NewConsistentHash 初始化并返回一个新的哈希环实例。
func NewConsistentHash(replicas int, fn Hash) *ConsistentHash {
	ch := &ConsistentHash{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[int]string),
	}
	if ch.hash == nil {
		ch.hash = crc32.ChecksumIEEE
	}
	slog.Info("consistent_hash initialized", "replicas", replicas)
	return ch
}

// Add 将一个或多个物理节点加入哈希环。
func (ch *ConsistentHash) Add(nodes ...string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	// 预分配 buffer 以减少循环内的内存分配
	buf := make([]byte, 0, 64)

	for _, node := range nodes {
		for i := 0; i < ch.replicas; i++ {
			buf = buf[:0] // 重置 buffer
			buf = strconv.AppendInt(buf, int64(i), 10)
			buf = append(buf, node...)

			hash := int(ch.hash(buf))
			ch.keys = append(ch.keys, hash)
			ch.hashMap[hash] = node
		}
		slog.Info("node added to consistent_hash ring", "node", node, "virtual_nodes", ch.replicas)
	}
	sort.Ints(ch.keys)
}

// Get 根据输入的键名查找其在哈希环上顺时针方向最近的物理节点。
func (ch *ConsistentHash) Get(key string) string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.keys) == 0 {
		return ""
	}

	hash := int(ch.hash([]byte(key)))

	idx := sort.Search(len(ch.keys), func(i int) bool {
		return ch.keys[i] >= hash
	})

	if idx == len(ch.keys) {
		idx = 0
	}

	return ch.hashMap[ch.keys[idx]]
}

// Remove 将指定的物理节点及其所有虚拟节点从哈希环中安全移除。
func (ch *ConsistentHash) Remove(node string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	buf := make([]byte, 0, 64)

	for i := 0; i < ch.replicas; i++ {
		buf = buf[:0]
		buf = strconv.AppendInt(buf, int64(i), 10)
		buf = append(buf, node...)

		hash := int(ch.hash(buf))
		idx := sort.SearchInts(ch.keys, hash)
		if idx < len(ch.keys) && ch.keys[idx] == hash {
			ch.keys = append(ch.keys[:idx], ch.keys[idx+1:]...)
		}
		delete(ch.hashMap, hash)
	}
	slog.Info("node removed from consistent_hash ring", "node", node)
}
