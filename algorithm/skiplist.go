package algorithm

import (
	"math/rand"
	"sync"
	"time"
)

const (
	maxLevel    = 32   // 跳表的最大层数
	probability = 0.25 // 晋升到上一层的概率
)

// SkipListNode 跳表节点
type SkipListNode struct {
	key   float64
	value any
	next  []*SkipListNode // 存储每一层的下一个节点指针
}

// SkipList 跳表数据结构
type SkipList struct {
	mu     sync.RWMutex
	header *SkipListNode
	level  int
	size   int
	random *rand.Rand
}

// NewSkipList 创建一个新的跳表
func NewSkipList() *SkipList {
	return &SkipList{
		header: &SkipListNode{
			next: make([]*SkipListNode, maxLevel),
		},
		level:  1,
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// randomLevel 随机生成节点的层数
func (sl *SkipList) randomLevel() int {
	lvl := 1
	for sl.random.Float64() < probability && lvl < maxLevel {
		lvl++
	}
	return lvl
}

// Insert 插入或更新键值对
func (sl *SkipList) Insert(key float64, value any) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	update := make([]*SkipListNode, maxLevel)
	curr := sl.header

	// 1. 从最高层开始向下寻找插入位置
	for i := sl.level - 1; i >= 0; i-- {
		for curr.next[i] != nil && curr.next[i].key < key {
			curr = curr.next[i]
		}
		update[i] = curr
	}

	curr = curr.next[0]

	// 2. 如果键已存在，更新值
	if curr != nil && curr.key == key {
		curr.value = value
		return
	}

	// 3. 产生随机层数
	lvl := sl.randomLevel()
	if lvl > sl.level {
		for i := sl.level; i < lvl; i++ {
			update[i] = sl.header
		}
		sl.level = lvl
	}

	// 4. 创建新节点并插入
	newNode := &SkipListNode{
		key:   key,
		value: value,
		next:  make([]*SkipListNode, lvl),
	}

	for i := 0; i < lvl; i++ {
		newNode.next[i] = update[i].next[i]
		update[i].next[i] = newNode
	}
	sl.size++
}

// Search 查找键对应的值
func (sl *SkipList) Search(key float64) (any, bool) {
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
	return nil, false
}

// Delete 删除指定键
func (sl *SkipList) Delete(key float64) bool {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	update := make([]*SkipListNode, maxLevel)
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

	for i := 0; i < sl.level; i++ {
		if update[i].next[i] != curr {
			break
		}
		update[i].next[i] = curr.next[i]
	}

	for sl.level > 1 && sl.header.next[sl.level-1] == nil {
		sl.level--
	}

	sl.size--
	return true
}

// Size 返回跳表元素总数
func (sl *SkipList) Size() int {
	return sl.size
}

// Iterator 返回一个按顺序遍历的迭代器
func (sl *SkipList) Iterator() *SkipListIterator {
	return &SkipListIterator{
		curr: sl.header.next[0],
	}
}

type SkipListIterator struct {
	curr *SkipListNode
}

func (it *SkipListIterator) Next() (float64, any, bool) {
	if it.curr == nil {
		return 0, nil, false
	}
	resKey, resVal := it.curr.key, it.curr.value
	it.curr = it.curr.next[0]
	return resKey, resVal, true
}
