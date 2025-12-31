package algorithm

import (
	"container/list"
	"sync"
)

// ACNode AC自动机节点
type ACNode struct {
	children map[rune]*ACNode
	fail     *ACNode  // 失败指针
	patterns []string // 以该节点结尾的所有模式串
}

// AhoCorasick AC自动机
type AhoCorasick struct {
	root *ACNode
	mu   sync.RWMutex
}

// NewAhoCorasick 创建AC自动机
func NewAhoCorasick() *AhoCorasick {
	return &AhoCorasick{
		root: &ACNode{
			children: make(map[rune]*ACNode),
		},
	}
}

// AddPatterns 添加多个模式串
func (ac *AhoCorasick) AddPatterns(patterns ...string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	for _, p := range patterns {
		curr := ac.root
		for _, r := range p {
			if _, ok := curr.children[r]; !ok {
				curr.children[r] = &ACNode{
					children: make(map[rune]*ACNode),
				}
			}
			curr = curr.children[r]
		}
		curr.patterns = append(curr.patterns, p)
	}
}

// Build 构造失败指针
func (ac *AhoCorasick) Build() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	queue := list.New()

	// 第一层节点的失败指针指向根节点
	for _, child := range ac.root.children {
		child.fail = ac.root
		queue.PushBack(child)
	}

	// BFS 构造
	for queue.Len() > 0 {
		element := queue.Front()
		queue.Remove(element)
		u := element.Value.(*ACNode)

		for r, v := range u.children {
			f := u.fail
			for f != nil {
				if next, ok := f.children[r]; ok {
					v.fail = next
					break
				}
				f = f.fail
			}
			if v.fail == nil {
				v.fail = ac.root
			}
			// 优化：合并后缀模式串
			if len(v.fail.patterns) > 0 {
				v.patterns = append(v.patterns, v.fail.patterns...)
			}
			queue.PushBack(v)
		}
	}
}

// Match 在文本中搜索所有出现的模式串
func (ac *AhoCorasick) Match(text string) map[string][]int {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	results := make(map[string][]int)
	curr := ac.root

	for i, r := range text {
		// 沿着失败指针寻找匹配
		for {
			if next, ok := curr.children[r]; ok {
				curr = next
				break
			}
			if curr == ac.root {
				break
			}
			curr = curr.fail
		}

		// 收集匹配结果
		if len(curr.patterns) > 0 {
			for _, p := range curr.patterns {
				results[p] = append(results[p], i-len(p)+1)
			}
		}
	}

	return results
}

// Contains 检查文本中是否包含任何模式串 (高性能版，只查有无)
func (ac *AhoCorasick) Contains(text string) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	curr := ac.root
	for _, r := range text {
		for {
			if next, ok := curr.children[r]; ok {
				curr = next
				break
			}
			if curr == ac.root {
				break
			}
			curr = curr.fail
		}
		if len(curr.patterns) > 0 {
			return true
		}
	}
	return false
}
