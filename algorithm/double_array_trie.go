package algorithm

import (
	"slices"
	"sync"
)

// DoubleArrayTrie (DAT) 是一种压缩的字典树实现。
// 它使用两个数组（Base 和 Check）来表示 Trie，具有极高的查询效率和较小的内存占用。
// 适用场景：静态字典匹配、分词、敏感词过滤。
// 注意：本实现为静态构建模式，不支持构建后的动态插入。如需更新，请重新构建。
type DoubleArrayTrie struct {
	base  []int
	check []int
	mu    sync.RWMutex // 仅保护读取，构建通常是离线的
}

// NewDoubleArrayTrie 创建一个新的空 DAT
func NewDoubleArrayTrie() *DoubleArrayTrie {
	return &DoubleArrayTrie{
		base:  make([]int, 0),
		check: make([]int, 0),
	}
}

// Build 从一组字符串构建 DAT。
// keys 必须是唯一的。
func (dat *DoubleArrayTrie) Build(keys []string) error {
	dat.mu.Lock()
	defer dat.mu.Unlock()

	if len(keys) == 0 {
		return nil
	}

	// 1. 排序 Keys，这对构建效率至关重要
	slices.Sort(keys)

	// 2. 构建临时的标准 Trie
	root := newDATNode()
	for _, key := range keys {
		root.insert(key)
	}

	// 3. 将 Trie 转换为双数组结构
	// 初始大小估算：节点数 * 4
	allocSize := 1024 * 1024
	dat.base = make([]int, allocSize)
	dat.check = make([]int, allocSize)

	// 初始化 root 的 base
	dat.base[0] = 1
	dat.check[0] = 0 // root 的 check 通常设为 0

	// 使用 BFS 遍历 Trie 并填充数组
	// queue 存储 (datTrieNode, datIndex)
	type nodeState struct {
		node     *datTrieNode
		datIndex int
	}
	queue := []*nodeState{{node: root, datIndex: 0}}

	// 记录已使用的 check 位置，用于寻找空闲位置
	used := make([]bool, allocSize)
	used[0] = true

	// 优化：记录第一个空闲的 check 位置
	firstFree := 1
	for firstFree < len(dat.check) && dat.check[firstFree] != 0 {
		firstFree++
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		children := curr.node.sortedChildren()
		if len(children) == 0 {
			continue
		}

		// 寻找合适的 Base 值
		// 使得对于所有子节点 c，check[base + c.code] == 0 (表示空闲)
		// 优化：利用 firstFree 快速定位可能的 base
		// base + children[0].code >= firstFree  =>  base >= firstFree - children[0].code
		minBase := 1
		if val := firstFree - int(children[0].code); val > 1 {
			minBase = val
		}

		base := minBase
		for {
			valid := true
			for _, child := range children {
				idx := base + int(child.code)
				// 自动扩容
				if idx >= len(dat.check) {
					newSize := len(dat.check) * 2
					for newSize <= idx {
						newSize *= 2
					}
					dat.resize(newSize)
					// 扩容后 used 也要扩容
					newUsed := make([]bool, newSize)
					copy(newUsed, used)
					used = newUsed
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

		// 确定了 Base，写入 DAT
		dat.base[curr.datIndex] = base
		used[curr.datIndex] = true

		for _, child := range children {
			childIdx := base + int(child.code)
			dat.check[childIdx] = curr.datIndex
			// 标记被占用
			// 如果占用了 firstFree，则需要更新 firstFree
			if childIdx == firstFree {
				for firstFree < len(dat.check) && dat.check[firstFree] != 0 {
					firstFree++
				}
			}

			// 如果是叶子节点（单词结尾），通常用 base 存储负值来标记
			// 这里我们简单地用一个独立的方式标记，或者假设所有节点都是有效路径
			// 为了支持 ExactMatch，我们需要标记结束。
			// 常见做法：把 Base 设为负数表示结尾。
			// 但因为 Base 还要用于子节点索引，所以如果一个节点既是结尾又有子节点，这种方法会有歧义。
			// 标准 DAT 处理这个问题通常是在每个字符串末尾加一个特殊字符（如 \0）。
			// 简化起见，我们在 Build 阶段给每个 key 加上 \0 (code=0) 处理。

			queue = append(queue, &nodeState{node: child.node, datIndex: childIdx})
		}
	}

	// 压缩数组（去掉末尾未使用的部分）
	dat.shrink()

	// 4. 释放临时 Trie
	releaseTrieNodes(root)

	return nil
}

func releaseTrieNodes(n *datTrieNode) {
	for _, child := range n.children {
		releaseTrieNodes(child)
	}
	n.reset()
	datTrieNodePool.Put(n)
}

// ExactMatch 执行双数组 Trie 的精确匹配。
func (dat *DoubleArrayTrie) ExactMatch(key string) bool {
	dat.mu.RLock()
	defer dat.mu.RUnlock()

	if len(dat.base) == 0 {
		return false
	}

	p := 0
	for _, ch := range key {
		code := int(ch)
		base := dat.base[p]
		next := base + code

		if next >= len(dat.check) || dat.check[next] != p {
			return false
		}
		p = next
	}

	// 最终检查结束符转移
	base := dat.base[p]
	next := base + 0
	return next < len(dat.check) && dat.check[next] == p
}

// CommonPrefixSearch 寻找给定 key 的所有公共前缀匹配。
// 例如：key="apple", 词典包含 "a", "app", "apple"，则返回这三个词。
func (dat *DoubleArrayTrie) CommonPrefixSearch(key string) []string {
	dat.mu.RLock()
	defer dat.mu.RUnlock()

	if len(dat.base) == 0 {
		return nil
	}

	var results []string
	p := 0 // root

	for i, ch := range key {
		// 1. 检查是否存在以当前节点结尾的完整词 (code=0)
		base := dat.base[p]
		endIdx := base + 0
		if endIdx < len(dat.check) && dat.check[endIdx] == p {
			results = append(results, key[:i])
		}

		// 2. 继续向下跳转
		code := int(ch)
		next := base + code
		if next >= len(dat.check) || dat.check[next] != p {
			return results
		}
		p = next
	}

	// 最终检查：key 自身是否也是一个完整词
	base := dat.base[p]
	endIdx := base + 0
	if endIdx < len(dat.check) && dat.check[endIdx] == p {
		results = append(results, key)
	}

	return results
}

// ... (existing helper methods stay same)
func (dat *DoubleArrayTrie) resize(newSize int) {
	newBase := make([]int, newSize)
	newCheck := make([]int, newSize)
	copy(newBase, dat.base)
	copy(newCheck, dat.check)
	dat.base = newBase
	dat.check = newCheck
}

// shrink 压缩数组到实际大小
func (dat *DoubleArrayTrie) shrink() {
	lastValid := 0
	for i := len(dat.check) - 1; i >= 0; i-- {
		if dat.check[i] != 0 || dat.base[i] != 0 {
			lastValid = i
			break
		}
	}
	newSize := lastValid + 1
	dat.base = dat.base[:newSize]
	dat.check = dat.check[:newSize]
}

// --- 辅助的内部 Trie 结构 ---

type datTrieNode struct {
	children map[rune]*datTrieNode
	isEnd    bool
}

var datTrieNodePool = sync.Pool{
	New: func() any {
		return &datTrieNode{children: make(map[rune]*datTrieNode)}
	},
}

func newDATNode() *datTrieNode {
	node := datTrieNodePool.Get().(*datTrieNode)
	return node
}

func (n *datTrieNode) reset() {
	for k := range n.children {
		delete(n.children, k)
	}
	n.isEnd = false
}

func (n *datTrieNode) insert(key string) {
	node := n
	for _, ch := range key {
		if ch == 0 {
			continue // rune(0) is reserved
		}
		if node.children[ch] == nil {
			node.children[ch] = newDATNode()
		}
		node = node.children[ch]
	}
	// 插入结束符节点，用 0 表示
	if node.children[0] == nil {
		node.children[0] = newDATNode()
	}
	node.children[0].isEnd = true
}

type childNode struct {
	code rune
	node *datTrieNode
}

func (n *datTrieNode) sortedChildren() []childNode {
	children := make([]childNode, 0, len(n.children))
	for k, v := range n.children {
		children = append(children, childNode{code: k, node: v})
	}
	slices.SortFunc(children, func(a, b childNode) int {
		if a.code < b.code {
			return -1
		}
		if a.code > b.code {
			return 1
		}
		return 0
	})
	return children
}
