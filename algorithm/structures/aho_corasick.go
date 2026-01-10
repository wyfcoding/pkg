package structures

import (
	"log/slog"
	"sync"
	"time"
)

const (
	initialQueueSize = 64
)

// ACNode AC自动机节点.
type ACNode struct {
	children   map[rune]*ACNode
	fail       *ACNode
	patternIdx []int
}

// AhoCorasick AC自动机.
type AhoCorasick struct {
	root     *ACNode
	patterns []string
	mu       sync.RWMutex
}

// NewAhoCorasick 创建AC自动机.
func NewAhoCorasick() *AhoCorasick {
	return &AhoCorasick{
		root: &ACNode{
			children:   make(map[rune]*ACNode),
			fail:       nil,
			patternIdx: nil,
		},
		patterns: make([]string, 0),
		mu:       sync.RWMutex{},
	}
}

// AddPatterns 添加多个模式串.
func (ac *AhoCorasick) AddPatterns(patterns ...string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	for _, p := range patterns {
		idx := len(ac.patterns)
		ac.patterns = append(ac.patterns, p)

		curr := ac.root
		for _, r := range p {
			if _, ok := curr.children[r]; !ok {
				curr.children[r] = &ACNode{
					children:   make(map[rune]*ACNode),
					fail:       nil,
					patternIdx: nil,
				}
			}

			curr = curr.children[r]
		}

		curr.patternIdx = append(curr.patternIdx, idx)
	}
}

// Build 构造失败指针.
func (ac *AhoCorasick) Build() {
	start := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()

	queue := make([]*ACNode, 0, initialQueueSize)

	for _, child := range ac.root.children {
		child.fail = ac.root
		queue = append(queue, child)
	}

	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]

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

			if len(v.fail.patternIdx) > 0 {
				v.patternIdx = append(v.patternIdx, v.fail.patternIdx...)
			}

			queue = append(queue, v)
		}
	}

	slog.Info("AhoCorasick build completed", "duration", time.Since(start))
}

// Match 在文本中搜索所有出现的模式串.
func (ac *AhoCorasick) Match(text string) map[string][]int {
	start := time.Now()
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	results := make(map[string][]int)
	curr := ac.root

	for i, r := range text {
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

		if len(curr.patternIdx) > 0 {
			for _, idx := range curr.patternIdx {
				p := ac.patterns[idx]
				results[p] = append(results[p], i-len(p)+1)
			}
		}
	}

	slog.Debug("AhoCorasick match completed",
		"text_len", len(text),
		"results_count", len(results),
		"duration", time.Since(start))

	return results
}

// Contains 检查文本中是否包含任何模式串.
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

		if len(curr.patternIdx) > 0 {
			return true
		}
	}

	return false
}
