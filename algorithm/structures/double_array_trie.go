package structures

import (
	"slices"
	"sync"
)

// DoubleArrayTrie (DAT) 是一种压缩的字典树泛型实现.
type DoubleArrayTrie[V any] struct {
	base   []int
	check  []int
	values []V
	mu     sync.RWMutex
}

const (
	initialAllocSize = 1024 * 1024
	rootBaseValue    = 1
	endRune          = 0
)

// NewDoubleArrayTrie 创建一个新的空 DAT.
func NewDoubleArrayTrie[V any]() *DoubleArrayTrie[V] {
	return &DoubleArrayTrie[V]{
		base:  make([]int, 0),
		check: make([]int, 0),
	}
}

// Build 从一组键值对构建 DAT.
func (dat *DoubleArrayTrie[V]) Build(keys []string, values []V) error {
	dat.mu.Lock()
	defer dat.mu.Unlock()

	if len(keys) == 0 {
		return nil
	}

	// 简单的包装，用于排序
	type entry struct {
		key string
		val V
	}
	entries := make([]entry, len(keys))
	for i := range keys {
		entries[i] = entry{keys[i], values[i]}
	}
	slices.SortFunc(entries, func(a, b entry) int {
		if a.key < b.key {
			return -1
		}
		if a.key > b.key {
			return 1
		}
		return 0
	})

	root := &datTrieNode[V]{children: make(map[rune]*datTrieNode[V])}
	for _, e := range entries {
		root.insert(e.key, e.val)
	}

	dat.base = make([]int, initialAllocSize)
	dat.check = make([]int, initialAllocSize)
	dat.values = make([]V, initialAllocSize)
	dat.base[0] = rootBaseValue
	dat.check[0] = 0

	dat.buildFromTrie(root)
	dat.shrink()

	return nil
}

type nodeState[V any] struct {
	node     *datTrieNode[V]
	datIndex int
}

func (dat *DoubleArrayTrie[V]) buildFromTrie(root *datTrieNode[V]) {
	queue := []*nodeState[V]{{node: root, datIndex: 0}}
	firstFree := 1

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		children := curr.node.sortedChildren()
		if len(children) == 0 {
			continue
		}

		base := dat.findBase(children, firstFree)
		dat.base[curr.datIndex] = base

		for i := range children {
			child := &children[i]
			childIdx := base + int(child.code)
			dat.check[childIdx] = curr.datIndex

			if child.node.isEnd {
				dat.values[childIdx] = child.node.value
			}

			if childIdx == firstFree {
				for firstFree < len(dat.check) && dat.check[firstFree] != 0 {
					firstFree++
				}
			}

			queue = append(queue, &nodeState[V]{node: child.node, datIndex: childIdx})
		}
	}
}

func (dat *DoubleArrayTrie[V]) findBase(children []childNode[V], firstFree int) int {
	minBase := 1
	if val := firstFree - int(children[0].code); val > 1 {
		minBase = val
	}

	base := minBase
	for {
		valid := true
		for i := range children {
			idx := base + int(children[i].code)
			if idx >= len(dat.check) {
				dat.grow(idx + 1)
			}

			if dat.check[idx] != 0 {
				valid = false
				break
			}
		}

		if valid {
			break
		}

		base++
	}

	return base
}

func (dat *DoubleArrayTrie[V]) grow(minSize int) {
	newSize := len(dat.check) * 2
	for newSize < minSize {
		newSize *= 2
	}

	nb := make([]int, newSize)
	nc := make([]int, newSize)
	nv := make([]V, newSize)
	copy(nb, dat.base)
	copy(nc, dat.check)
	copy(nv, dat.values)
	dat.base = nb
	dat.check = nc
	dat.values = nv
}

// ExactMatch 执行双数组 Trie 的精确匹配，返回关联的值。
func (dat *DoubleArrayTrie[V]) ExactMatch(key string) (V, bool) {
	dat.mu.RLock()
	defer dat.mu.RUnlock()

	var zero V
	if len(dat.base) == 0 {
		return zero, false
	}

	p := 0
	for _, ch := range key {
		code := int(ch)
		next := dat.base[p] + code

		if next >= len(dat.check) || dat.check[next] != p {
			return zero, false
		}

		p = next
	}

	endIdx := dat.base[p] + endRune
	if endIdx < len(dat.check) && dat.check[endIdx] == p {
		return dat.values[endIdx], true
	}

	return zero, false
}

func (dat *DoubleArrayTrie[V]) shrink() {
	last := 0
	for i := len(dat.check) - 1; i >= 0; i-- {
		if dat.check[i] != 0 || dat.base[i] != 0 {
			last = i
			break
		}
	}

	size := last + 1
	dat.base = dat.base[:size]
	dat.check = dat.check[:size]
	dat.values = dat.values[:size]
}

type datTrieNode[V any] struct {
	children map[rune]*datTrieNode[V]
	value    V
	isEnd    bool
}

func (n *datTrieNode[V]) insert(key string, val V) {
	curr := n
	for _, ch := range key {
		if ch == endRune {
			continue
		}

		if curr.children[ch] == nil {
			curr.children[ch] = &datTrieNode[V]{children: make(map[rune]*datTrieNode[V])}
		}

		curr = curr.children[ch]
	}

	if curr.children[endRune] == nil {
		curr.children[endRune] = &datTrieNode[V]{children: make(map[rune]*datTrieNode[V])}
	}

	curr.children[endRune].isEnd = true
	curr.children[endRune].value = val
}

type childNode[V any] struct {
	node *datTrieNode[V]
	code rune
}

func (n *datTrieNode[V]) sortedChildren() []childNode[V] {
	res := make([]childNode[V], 0, len(n.children))
	for k, v := range n.children {
		res = append(res, childNode[V]{code: k, node: v})
	}

	slices.SortFunc(res, func(a, b childNode[V]) int {
		if a.code < b.code {
			return -1
		}
		if a.code > b.code {
			return 1
		}
		return 0
	})

	return res
}
