package algorithm

import (
	"slices"
	"sync"
)

// DoubleArrayTrie (DAT) 是一种压缩的字典树实现.
type DoubleArrayTrie struct {
	base  []int
	check []int
	mu    sync.RWMutex
}

const (
	initialAllocSize = 1024 * 1024
	rootBaseValue    = 1
	endRune          = 0
)

// NewDoubleArrayTrie 创建一个新的空 DAT.
func NewDoubleArrayTrie() *DoubleArrayTrie {
	return &DoubleArrayTrie{
		base:  make([]int, 0),
		check: make([]int, 0),
		mu:    sync.RWMutex{},
	}
}

// Build 从一组字符串构建 DAT.
func (dat *DoubleArrayTrie) Build(keys []string) error {
	dat.mu.Lock()
	defer dat.mu.Unlock()

	if len(keys) == 0 {
		return nil
	}

	slices.Sort(keys)

	root := newDATNode()
	for _, key := range keys {
		root.insert(key)
	}

	dat.base = make([]int, initialAllocSize)
	dat.check = make([]int, initialAllocSize)
	dat.base[0] = rootBaseValue
	dat.check[0] = 0

	used := make([]bool, initialAllocSize)
	used[0] = true

	dat.buildFromTrie(root, used)
	dat.shrink()
	releaseTrieNodes(root)

	return nil
}

type nodeState struct {
	node     *datTrieNode
	datIndex int
}

func (dat *DoubleArrayTrie) buildFromTrie(root *datTrieNode, used []bool) {
	queue := []*nodeState{{node: root, datIndex: 0}}
	firstFree := 1

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		children := curr.node.sortedChildren()
		if len(children) == 0 {
			continue
		}

		base := dat.findBase(children, firstFree, used)
		dat.base[curr.datIndex] = base
		used[curr.datIndex] = true

		for i := range children {
			child := &children[i]
			childIdx := base + int(child.code)
			dat.check[childIdx] = curr.datIndex

			if childIdx == firstFree {
				for firstFree < len(dat.check) && dat.check[firstFree] != 0 {
					firstFree++
				}
			}

			queue = append(queue, &nodeState{node: child.node, datIndex: childIdx})
		}
	}
}

func (dat *DoubleArrayTrie) findBase(children []childNode, firstFree int, used []bool) int {
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

func (dat *DoubleArrayTrie) grow(minSize int) {
	newSize := len(dat.check) * 2
	for newSize < minSize {
		newSize *= 2
	}

	nb := make([]int, newSize)
	nc := make([]int, newSize)
	copy(nb, dat.base)
	copy(nc, dat.check)
	dat.base = nb
	dat.check = nc
}

func releaseTrieNodes(n *datTrieNode) {
	for _, child := range n.children {
		releaseTrieNodes(child)
	}

	n.reset()
	datTrieNodePool.Put(n)
}

// ExactMatch 执行双数组 Trie 的精确匹配.
func (dat *DoubleArrayTrie) ExactMatch(key string) bool {
	dat.mu.RLock()
	defer dat.mu.RUnlock()

	if len(dat.base) == 0 {
		return false
	}

	p := 0
	for _, ch := range key {
		code := int(ch)
		next := dat.base[p] + code

		if next >= len(dat.check) || dat.check[next] != p {
			return false
		}

		p = next
	}

	endIdx := dat.base[p] + endRune

	return endIdx < len(dat.check) && dat.check[endIdx] == p
}

// CommonPrefixSearch 寻找给定 key 的所有公共前缀匹配.
func (dat *DoubleArrayTrie) CommonPrefixSearch(key string) []string {
	dat.mu.RLock()
	defer dat.mu.RUnlock()

	if len(dat.base) == 0 {
		return nil
	}

	var results []string
	p := 0

	for i, ch := range key {
		base := dat.base[p]
		endIdx := base + endRune
		if endIdx < len(dat.check) && dat.check[endIdx] == p {
			results = append(results, key[:i])
		}

		code := int(ch)
		next := base + code
		if next >= len(dat.check) || dat.check[next] != p {
			return results
		}

		p = next
	}

	base := dat.base[p]
	endIdx := base + endRune
	if endIdx < len(dat.check) && dat.check[endIdx] == p {
		results = append(results, key)
	}

	return results
}

func (dat *DoubleArrayTrie) shrink() {
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
}

type datTrieNode struct {
	children map[rune]*datTrieNode
	isEnd    bool
}

var datTrieNodePool = sync.Pool{
	New: func() any {
		return &datTrieNode{children: make(map[rune]*datTrieNode), isEnd: false}
	},
}

func newDATNode() *datTrieNode {
	node, ok := datTrieNodePool.Get().(*datTrieNode)
	if !ok {
		return &datTrieNode{children: make(map[rune]*datTrieNode), isEnd: false}
	}

	return node
}

func (n *datTrieNode) reset() {
	for k := range n.children {
		delete(n.children, k)
	}

	n.isEnd = false
}

func (n *datTrieNode) insert(key string) {
	curr := n
	for _, ch := range key {
		if ch == endRune {
			continue
		}

		if curr.children[ch] == nil {
			curr.children[ch] = newDATNode()
		}

		curr = curr.children[ch]
	}

	if curr.children[endRune] == nil {
		curr.children[endRune] = newDATNode()
	}

	curr.children[endRune].isEnd = true
}

type childNode struct {
	node *datTrieNode
	code rune
}

func (n *datTrieNode) sortedChildren() []childNode {
	res := make([]childNode, 0, len(n.children))
	for k, v := range n.children {
		res = append(res, childNode{code: k, node: v})
	}

	slices.SortFunc(res, func(a, b childNode) int {
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
