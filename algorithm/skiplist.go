package algorithm

import (
	"cmp"
	"math/rand"
	"sync"
	"time"
)

const (
	maxLevel    = 32   // 跳表的最大层数，足以支撑 2^32 个元素
	probability = 0.25 // 晋升到上一层的概率 (P=1/4 均衡了查询效率与空间开销)
)

// SkipListNode 跳表节点，使用泛型支持有序 Key。
type SkipListNode[K cmp.Ordered, V any] struct {
	key   K
	value V
	next  []*SkipListNode[K, V] // 存储每一层的下一个节点指针
}

// SkipList 高性能泛型跳表实现。
// 复杂度分析：
// - 查询: 平均 O(log N), 最坏 O(N)
// - 插入: 平均 O(log N), 最坏 O(N)
// - 删除: 平均 O(log N), 最坏 O(N)
// - 空间: O(N)
type SkipList[K cmp.Ordered, V any] struct {
	mu     sync.RWMutex
	header *SkipListNode[K, V]
	level  int
	size   int
	random *rand.Rand
	// 缓存随机数生成器以减少锁竞争（在更高并发下建议使用 Pool）
	randMu sync.Mutex
}

// NewSkipList 创建一个新的跳表。
func NewSkipList[K cmp.Ordered, V any]() *SkipList[K, V] {
	return &SkipList[K, V]{
		header: &SkipListNode[K, V]{
			next: make([]*SkipListNode[K, V], maxLevel),
		},
		level:  1,
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// randomLevel 随机生成节点的层数，采用更高效的算法。
func (sl *SkipList[K, V]) randomLevel() int {
	sl.randMu.Lock()
	defer sl.randMu.Unlock()
	lvl := 1
	for sl.random.Float64() < probability && lvl < maxLevel {
		lvl++
	}
	return lvl
}

// Insert 插入或更新键值对。
func (sl *SkipList[K, V]) Insert(key K, value V) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	update := make([]*SkipListNode[K, V], maxLevel)
	curr := sl.header

	// 1. 从最高有效层向下搜寻插入位置
	for i := sl.level - 1; i >= 0; i-- {
		for curr.next[i] != nil && curr.next[i].key < key {
			curr = curr.next[i]
		}
		update[i] = curr
	}

	curr = curr.next[0]

	// 2. 如果键已存在，直接更新 Value
	if curr != nil && curr.key == key {
		curr.value = value
		return
	}

	// 3. 决定新节点层数
	lvl := sl.randomLevel()
	if lvl > sl.level {
		for i := sl.level; i < lvl; i++ {
			update[i] = sl.header
		}
		sl.level = lvl
	}

	// 4. 执行链表插入
	newNode := &SkipListNode[K, V]{
		key:   key,
		value: value,
		next:  make([]*SkipListNode[K, V], lvl),
	}

	for i := 0; i < lvl; i++ {
		newNode.next[i] = update[i].next[i]
		update[i].next[i] = newNode
	}
	sl.size++
}

// Search 根据 Key 查找 Value。
func (sl *SkipList[K, V]) Search(key K) (V, bool) {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	curr := sl.header
	for i := sl.level - 1; i >= 0; i-- {
		for curr.next[i] != nil && curr.next[i].key < key {
			curr = curr.next[i]
		}
	}

	curr = curr.next[0]
	if curr != nil && curr.key == key {
		return curr.value, true
	}
	var zero V
	return zero, false
}

// Delete 删除 Key，返回是否成功。
func (sl *SkipList[K, V]) Delete(key K) bool {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	update := make([]*SkipListNode[K, V], maxLevel)
	curr := sl.header

	for i := sl.level - 1; i >= 0; i-- {
		for curr.next[i] != nil && curr.next[i].key < key {
			curr = curr.next[i]
		}
		update[i] = curr
	}

	curr = curr.next[0]
	if curr == nil || curr.key != key {
		return false
	}

	// 移除每一层的链接
	for i := 0; i < sl.level; i++ {
		if update[i].next[i] != curr {
			break
		}
		update[i].next[i] = curr.next[i]
	}

	// 更新跳表当前的最大层数
	for sl.level > 1 && sl.header.next[sl.level-1] == nil {
		sl.level--
	}

	sl.size--
	return true
}

// Size 返回当前元素个数。
func (sl *SkipList[K, V]) Size() int {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.size
}

// Iterator 返回顺序遍历器。
func (sl *SkipList[K, V]) Iterator() *SkipListIterator[K, V] {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return &SkipListIterator[K, V]{
		curr: sl.header.next[0],
	}
}

type SkipListIterator[K cmp.Ordered, V any] struct {
	curr *SkipListNode[K, V]
}

func (it *SkipListIterator[K, V]) Next() (K, V, bool) {
	if it.curr == nil {
		var zk K
		var zv V
		return zk, zv, false
	}
	resKey, resVal := it.curr.key, it.curr.value
	it.curr = it.curr.next[0]
	return resKey, resVal, true
}
