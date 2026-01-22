package structures

import (
	"sync"
)

// TrieNode 结构体代表字典树中的一个节点。
type TrieNode[V any] struct {
	children map[rune]*TrieNode[V]
	value    V
	isEnd    bool
}

// Reset 重置节点以便复用。
func (n *TrieNode[V]) reset() {
	for k := range n.children {
		delete(n.children, k)
	}
	n.isEnd = false
	var zero V
	n.value = zero
}

// Trie 结构体实现了字典树（前缀树）数据结构。
type Trie[V any] struct {
	root *TrieNode[V]
	mu   sync.RWMutex
}

// NewTrie 创建并返回一个新的 Trie 实例。
func NewTrie[V any]() *Trie[V] {
	return &Trie[V]{
		root: &TrieNode[V]{
			children: make(map[rune]*TrieNode[V]),
		},
	}
}

// Insert 将一个单词及其关联的值插入到字典树中。
func (t *Trie[V]) Insert(word string, value V) {
	t.mu.Lock()
	defer t.mu.Unlock()

	node := t.root
	for _, ch := range word {
		if node.children[ch] == nil {
			node.children[ch] = &TrieNode[V]{
				children: make(map[rune]*TrieNode[V]),
			}
		}
		node = node.children[ch]
	}
	node.isEnd = true
	node.value = value
}

// Search 精确搜索字典树中是否存在指定的单词，并返回其关联的值。
func (t *Trie[V]) Search(word string) (V, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node := t.root
	for _, ch := range word {
		if node.children[ch] == nil {
			var zero V
			return zero, false
		}
		node = node.children[ch]
	}
	return node.value, node.isEnd
}

// Remove 从字典树中移除一个单词。
func (t *Trie[V]) Remove(word string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.remove(t.root, word, 0)
}

// remove 递归删除辅助函数。
func (t *Trie[V]) remove(node *TrieNode[V], word string, depth int) bool {
	if depth == len(word) {
		if !node.isEnd {
			return false // 单词不存在。
		}
		node.isEnd = false
		var zero V
		node.value = zero
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
		return !node.isEnd && len(node.children) == 0
	}

	return false
}

// StartsWith 查找所有以给定前缀开头的单词所关联的值。
func (t *Trie[V]) StartsWith(prefix string) []V {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node := t.root
	for _, ch := range prefix {
		if node.children[ch] == nil {
			return nil
		}
		node = node.children[ch]
	}

	results := make([]V, 0)
	t.dfs(node, &results)
	return results
}

// dfs (深度优先搜索) 辅助函数。
func (t *Trie[V]) dfs(node *TrieNode[V], results *[]V) {
	if node.isEnd {
		*results = append(*results, node.value)
	}

	for _, child := range node.children {
		t.dfs(child, results)
	}
}
