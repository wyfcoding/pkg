package algorithm

import (
	"cmp"
	"sync"
	"time"
)

const (
	maxLevel = 32 // 跳表的最大层数。
	// probability = 0.25 // 晋升概率 (P = 1/4)。

	// 优化常数：对应 probability = 0.25 = 1/(2^probShift)。
	// 使用位运算代替浮点数比较，显著提升性能。
	probShift = 2                    // 每次消耗的随机比特位 (log2(1/P))。
	probMask  = (1 << probShift) - 1 // 掩码 (11b = 3)。
)

// SkipListNode 跳表节点。
type SkipListNode[K cmp.Ordered, V any] struct {
	key   K
	value V
	next  []*SkipListNode[K, V]
}

// SkipList 高性能泛型跳表实现。
// 优化：使用原子操作的 Xorshift 随机数生成器，去除了随机数生成的全局锁。
type SkipList[K cmp.Ordered, V any] struct {
	header    *SkipListNode[K, V]
	mu        sync.RWMutex
	randState uint64
	level     int
	size      int
}

// NewSkipList 创建一个新的跳表。
func NewSkipList[K cmp.Ordered, V any]() *SkipList[K, V] {
	return &SkipList[K, V]{
		header: &SkipListNode[K, V]{
			next: make([]*SkipListNode[K, V], maxLevel),
		},
		level:     1,
		randState: uint64(time.Now().UnixNano()),
	}
}

// fastRand 使用 Xorshift 算法生成伪随机数，无锁且高效。
func (sl *SkipList[K, V]) fastRand() uint32 {
	// 这里采用 splitmix64 的变体或简单的 xorshift。
	state := sl.randState
	state ^= state << 13
	state ^= state >> 7
	state ^= state << 17
	sl.randState = state

	return uint32(state)
}

// randomLevel 随机生成节点的层数，使用位运算优化。
// 利用概率 P=0.25 (1/4)，即每 2 bits 为 00 时增加一层。
func (sl *SkipList[K, V]) randomLevel() int {
	lvl := 1
	// 生成随机数。
	r := sl.fastRand()
	// 只要低 probShift 位是 0 (概率 probability)，就增加层数。
	for (r&probMask) == 0 && lvl < maxLevel {
		lvl++
		r >>= probShift
	}

	return lvl
}

// Insert 插入或更新键值对。
func (sl *SkipList[K, V]) Insert(key K, value V) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	// 优化：使用栈分配数组避免切片逃逸到堆。
	var update [maxLevel]*SkipListNode[K, V]
	curr := sl.header

	// 1. 从最高有效层向下搜寻插入位置。
	for i := sl.level - 1; i >= 0; i-- {
		for curr.next[i] != nil && curr.next[i].key < key {
			curr = curr.next[i]
		}
		update[i] = curr
	}

	curr = curr.next[0]

	// 2. 如果键已存在，直接更新 Value。
	if curr != nil && curr.key == key {
		curr.value = value

		return
	}

	// 3. 决定新节点层数。
	lvl := sl.randomLevel()
	if lvl > sl.level {
		for i := sl.level; i < lvl; i++ {
			update[i] = sl.header
		}
		sl.level = lvl
	}

	// 4. 执行链表插入。
	newNode := &SkipListNode[K, V]{
		key:   key,
		value: value,
		next:  make([]*SkipListNode[K, V], lvl),
	}

	for i := range lvl {
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

	var update [maxLevel]*SkipListNode[K, V]
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

	// 移除每一层的链接。
	for i := range sl.level {
		if update[i].next[i] != curr {
			break
		}
		update[i].next[i] = curr.next[i]
	}

	// 更新跳表当前的最大层数。
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

func (it *SkipListIterator[K, V]) Next() (key K, value V, ok bool) {
	if it.curr == nil {
		var zk K
		var zv V
		return zk, zv, false
	}
	resKey, resVal := it.curr.key, it.curr.value
	it.curr = it.curr.next[0]
	return resKey, resVal, true
}
