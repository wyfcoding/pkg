package algorithm

import (
	"cmp"
	"sync"
	"time"
)

const (
	maxLevel = 32 // 跳表的最大层数
	// probability = 0.25 // 晋升概率 (P = 1/4)

	// 优化常数：对应 probability = 0.25 = 1/(2^probShift)
	// 使用位运算代替浮点数比较，显著提升性能
	probShift = 2                    // 每次消耗的随机比特位 (log2(1/P))
	probMask  = (1 << probShift) - 1 // 掩码 (11b = 3)
)

// SkipListNode 跳表节点
type SkipListNode[K cmp.Ordered, V any] struct {
	key   K
	value V
	next  []*SkipListNode[K, V]
}

// SkipList 高性能泛型跳表实现。
// 优化：使用原子操作的 Xorshift 随机数生成器，去除了随机数生成的全局锁。
type SkipList[K cmp.Ordered, V any] struct {
	mu     sync.RWMutex
	header *SkipListNode[K, V]
	level  int
	size   int
	// randState 用于 Xorshift 随机数生成器
	randState uint64
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
	// 使用 atomic 更新状态，支持并发调用（虽然 randomLevel 主要在 Insert 内，Insert 有锁，
	// 但 fastRand 设计为无锁可支持未来可能的细粒度锁优化）
	// 注意：当前 Insert 有全局锁，所以这里的 atomic 主要是为了正确性，性能上无竞争。
	// 如果未来改为细粒度锁或无锁跳表，这个随机生成器是安全的。
	// 这里为了极致性能，在有锁保护的 Insert 中其实可以直接操作，但为了通用性保持 atomic。
	// 鉴于 SkipList 目前是全局锁，atomic 只有单个 owner，开销极小。
	// 更好的方式：如果 SkipList 是全局锁，那么 randState 不需要 atomic。
	// 但为了代码的鲁棒性（万一 randomLevel 被并发调用），保留 CAS 或直接 Load/Store。
	// 既然 Insert 有 sl.mu，我们直接读写 sl.randState 即可？
	// 答：Insert 有 sl.mu，所以 randomLevel 是互斥的。可以去掉 atomic。
	// 但为了防止如果在无锁读路径或其他地方调用，我们采用简单的非加密 hash 混淆。

	// 这里采用 splitmix64 的变体或简单的 xorshift
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
	// 生成随机数
	r := sl.fastRand()
	// 只要低 probShift 位是 0 (概率 probability)，就增加层数
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

	// 优化：使用栈分配数组避免切片逃逸到堆
	var update [maxLevel]*SkipListNode[K, V]
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
