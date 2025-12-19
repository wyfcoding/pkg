package algorithm

import (
	"sync"
)

// TrieNode 结构体代表字典树中的一个节点。
// 每个节点可以有多个子节点，对应着字符串中的下一个字符。
type TrieNode struct {
	children map[rune]*TrieNode // 指向子节点的map，键是字符（rune），值是对应的子节点。
	isEnd    bool               // 标记是否是某个单词的结尾。
	value    interface{}        // 如果是单词结尾，可以存储与该单词相关联的值。
}

// Trie 结构体实现了字典树（前缀树）数据结构。
// 字典树用于高效地存储和检索字符串集合，尤其擅长处理前缀搜索、自动完成和拼写检查等场景。
type Trie struct {
	root *TrieNode    // 字典树的根节点，不代表任何字符。
	mu   sync.RWMutex // 读写锁，用于保护字典树的并发访问。
}

// NewTrie 创建并返回一个新的 Trie 实例。
func NewTrie() *Trie {
	return &Trie{
		root: &TrieNode{ // 初始化根节点。
			children: make(map[rune]*TrieNode),
		},
	}
}

// Insert 将一个单词及其关联的值插入到字典树中。
// word: 要插入的单词。
// value: 与该单词关联的值。
func (t *Trie) Insert(word string, value interface{}) {
	t.mu.Lock()         // 加写锁，以确保插入操作的线程安全。
	defer t.mu.Unlock() // 确保函数退出时解锁。

	node := t.root            // 从根节点开始遍历。
	for _, ch := range word { // 遍历单词中的每一个字符。
		if node.children[ch] == nil {
			// 如果当前字符没有对应的子节点，则创建一个新节点。
			node.children[ch] = &TrieNode{
				children: make(map[rune]*TrieNode),
			}
		}
		node = node.children[ch] // 移动到下一个节点。
	}
	node.isEnd = true  // 标记当前节点是单词的结尾。
	node.value = value // 存储与单词关联的值。
}

// Search 精确搜索字典树中是否存在指定的单词，并返回其关联的值。
// word: 要搜索的单词。
// 返回值：
//   - interface{}: 如果找到单词，返回其关联的值；否则返回 nil。
//   - bool: 如果找到单词且 `isEnd` 为 true，则返回 true；否则返回 false。
func (t *Trie) Search(word string) (interface{}, bool) {
	t.mu.RLock()         // 搜索操作只需要读锁。
	defer t.mu.RUnlock() // 确保函数退出时解锁。

	node := t.root            // 从根节点开始遍历。
	for _, ch := range word { // 遍历单词中的每一个字符。
		if node.children[ch] == nil {
			return nil, false // 如果路径中断，则单词不存在。
		}
		node = node.children[ch] // 移动到下一个节点。
	}
	return node.value, node.isEnd // 返回找到节点的值和isEnd状态。
}

// StartsWith 查找所有以给定前缀开头的单词所关联的值。
// 应用场景：商品名称自动完成、搜索建议、敏感词过滤等。
// prefix: 要搜索的前缀字符串。
// 返回所有匹配前缀的单词所关联的值的切片。
func (t *Trie) StartsWith(prefix string) []interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node := t.root // 从根节点开始，沿着前缀路径遍历。
	for _, ch := range prefix {
		if node.children[ch] == nil {
			return nil // 如果前缀路径中断，则没有以该前缀开头的单词。
		}
		node = node.children[ch] // 移动到前缀的最后一个字符对应的节点。
	}

	results := make([]interface{}, 0)
	t.dfs(node, &results) // 从前缀的最后一个字符节点开始，进行深度优先搜索，收集所有以该前缀开头的单词。
	return results
}

// dfs (深度优先搜索) 辅助函数，用于从给定的TrieNode开始，递归地收集所有以该节点为前缀的单词的值。
// node: 当前遍历到的TrieNode。
// results: 用于存储收集到的值的切片指针。
func (t *Trie) dfs(node *TrieNode, results *[]interface{}) {
	if node.isEnd {
		*results = append(*results, node.value) // 如果当前节点是单词结尾，则将其值添加到结果中。
	}

	// 递归遍历所有子节点。
	for _, child := range node.children {
		t.dfs(child, results)
	}
}
