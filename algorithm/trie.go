package algorithm

import (
	"sync"
)

// TrieNode 结构体代表字典树中的一个节点。
type TrieNode struct {
	children map[rune]*TrieNode
	isEnd    bool
	value    any
}

var trieNodePool = sync.Pool{
	New: func() any {
		return &TrieNode{
			children: make(map[rune]*TrieNode),
		}
	},
}

func newTrieNode() *TrieNode {
	node := trieNodePool.Get().(*TrieNode)
	return node
}

// Reset 重置节点以便复用。
func (n *TrieNode) reset() {
	for k := range n.children {
		delete(n.children, k)
	}
	n.isEnd = false
	n.value = nil
}

// Trie 结构体实现了字典树（前缀树）数据结构。
type Trie struct {
	root *TrieNode
	mu   sync.RWMutex
}

// NewTrie 创建并返回一个新的 Trie 实例。
func NewTrie() *Trie {
	return &Trie{
		root: newTrieNode(),
	}
}

// Insert 将一个单词及其关联的值插入到字典树中。
func (t *Trie) Insert(word string, value any) {
	t.mu.Lock()
	defer t.mu.Unlock()

	node := t.root
	for _, ch := range word {
		if node.children[ch] == nil {
			node.children[ch] = newTrieNode()
		}
		node = node.children[ch]
	}
	node.isEnd = true
	node.value = value
}

// Search 精确搜索字典树中是否存在指定的单词，并返回其关联的值。
func (t *Trie) Search(word string) (any, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node := t.root
	for _, ch := range word {
		if node.children[ch] == nil {
			return nil, false
		}
		node = node.children[ch]
	}
	return node.value, node.isEnd
}

// Remove 从字典树中移除一个单词。
// 如果节点不再被使用（无子节点且不是单词结尾），它将被回收到 sync.Pool 中以减少内存压力。
func (t *Trie) Remove(word string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.remove(t.root, word, 0)
}

// remove 递归删除辅助函数。
func (t *Trie) remove(node *TrieNode, word string, depth int) bool {
	if depth == len(word) {
		if !node.isEnd {
			return false // 单词不存在。
		}
		node.isEnd = false
		node.value = nil
		// 如果该节点没有子节点，说明可以被移除。
		return len(node.children) == 0
	}

	ch := rune(word[depth])
	child, ok := node.children[ch]
	if !ok {
		return false // 路径中断，单词不存在。
	}

	shouldDeleteChild := t.remove(child, word, depth+1)

	if shouldDeleteChild {
		delete(node.children, ch)
		// 回收子节点到对象池。
		child.reset()
		trieNodePool.Put(child)

		// 检查当前节点是否也需要被移除。
		// 1. 不是其他单词的结尾。
		// 2. 没有其他子节点。
		return !node.isEnd && len(node.children) == 0
	}

	return false
}

// StartsWith 查找所有以给定前缀开头的单词所关联的值。
// 应用场景：商品名称自动完成、搜索建议、敏感词过滤等。
// prefix: 要搜索的前缀字符串。
// 返回所有匹配前缀的单词所关联的值的切片。
func (t *Trie) StartsWith(prefix string) []any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node := t.root // 从根节点开始，沿着前缀路径遍历。
	for _, ch := range prefix {
		if node.children[ch] == nil {
			return nil // 如果前缀路径中断，则没有以该前缀开头的单词。
		}
		node = node.children[ch] // 移动到前缀的最后一个字符对应的节点。
	}

	results := make([]any, 0)
	t.dfs(node, &results) // 从前缀的最后一个字符节点开始，进行深度优先搜索，收集所有以该前缀开头的单词。
	return results
}

// dfs (深度优先搜索) 辅助函数，用于从给定的TrieNode开始，递归地收集所有以该节点为前缀的单词的值。
// node: 当前遍历到的TrieNode。
// results: 用于存储收集到的值的切片指针。
func (t *Trie) dfs(node *TrieNode, results *[]any) {
	if node.isEnd {
		*results = append(*results, node.value) // 如果当前节点是单词结尾，则将其值添加到结果中。
	}

	// 递归遍历所有子节点。
	for _, child := range node.children {
		t.dfs(child, results)
	}
}
