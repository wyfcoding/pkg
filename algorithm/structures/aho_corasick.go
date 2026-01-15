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
type ACNode[T any] struct {
	children   map[rune]*ACNode[T]
	fail       *ACNode[T]
	endFail    *ACNode[T] // 指向最近的一个代表模式串终点的 fail 节点
	patternIdx int        // 如果当前节点是终点，记录模式串索引，否则为 -1
}

// AhoCorasick AC自动机泛型实现.
type AhoCorasick[T any] struct {
	root     *ACNode[T]
	patterns []T
	mu       sync.RWMutex
	toString func(T) string
}

// NewAhoCorasick 创建AC自动机.
func NewAhoCorasick[T any](toString func(T) string) *AhoCorasick[T] {
	return &AhoCorasick[T]{
		root: &ACNode[T]{
			children:   make(map[rune]*ACNode[T]),
			fail:       nil,
			endFail:    nil,
			patternIdx: -1,
		},
		patterns: make([]T, 0),
		mu:       sync.RWMutex{},
		toString: toString,
	}
}

// NewStringAhoCorasick 创建专门处理字符串的 AC 自动机助手.
func NewStringAhoCorasick() *AhoCorasick[string] {
	return NewAhoCorasick(func(s string) string { return s })
}

// AddPatterns 添加多个模式串.
func (ac *AhoCorasick[T]) AddPatterns(patterns ...T) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	for _, p := range patterns {
		idx := len(ac.patterns)
		ac.patterns = append(ac.patterns, p)

		curr := ac.root
		for _, r := range ac.toString(p) {
			if _, ok := curr.children[r]; !ok {
				curr.children[r] = &ACNode[T]{
					children:   make(map[rune]*ACNode[T]),
					fail:       nil,
					endFail:    nil,
					patternIdx: -1,
				}
			}

			curr = curr.children[r]
		}

		curr.patternIdx = idx
	}
}

// Build 构造失败指针.
func (ac *AhoCorasick[T]) Build() {
	start := time.Now()
	ac.mu.Lock()
	defer ac.mu.Unlock()

	queue := make([]*ACNode[T], 0, initialQueueSize)

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

			// 优化：计算 endFail 链，避免 slice 拷贝
			if v.fail.patternIdx != -1 {
				v.endFail = v.fail
			} else {
				v.endFail = v.fail.endFail
			}

			queue = append(queue, v)
		}
	}

	slog.Info("AhoCorasick build completed", "duration", time.Since(start))
}

// MatchResult 匹配结果单元.
type MatchResult[T any] struct {
	Pattern T
	Pos     int
}

// Match 在文本中搜索所有出现的模式串.
func (ac *AhoCorasick[T]) Match(text string) []MatchResult[T] {
	start := time.Now()
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var results []MatchResult[T]
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

		temp := curr
		if temp.patternIdx == -1 {
			temp = temp.endFail
		}

		for temp != nil {
			results = append(results, MatchResult[T]{
				Pattern: ac.patterns[temp.patternIdx],
				Pos:     i - len(ac.toString(ac.patterns[temp.patternIdx])) + 1,
			})
			temp = temp.endFail
		}
	}

	slog.Debug("AhoCorasick match completed",
		"text_len", len(text),
		"results_count", len(results),
		"duration", time.Since(start))

		return results

	}

	

	// Contains 检查文本中是否包含任何模式串.

	func (ac *AhoCorasick[T]) Contains(text string) bool {

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

	

			if curr.patternIdx != -1 || curr.endFail != nil {

				return true

			}

		}

	

		return false

	}

	