package algorithm

import (
	"sync"
)

// SuffixArray 结构体实现了后缀数组。
// 后缀数组是一个字符串所有后缀的排序数组，它在字符串匹配、模式查找等领域有广泛应用。
// 相较于后缀树，后缀数组在空间效率上通常更优。
type SuffixArray struct {
	text   string       // 原始字符串。
	sa     []int        // 后缀数组。
	rank   []int        // 排名数组。
	height []int        // LCP 数组：height[i] 表示 sa[i] 和 sa[i-1] 后缀的最长公共前缀。
	mu     sync.RWMutex // 读写锁。
}

// NewSuffixArray 创建并返回一个新的 SuffixArray 实例。
func NewSuffixArray(text string) *SuffixArray {
	n := len(text)
	sa := &SuffixArray{
		text:   text,
		sa:     make([]int, n),
		rank:   make([]int, n),
		height: make([]int, n),
	}
	sa.build()
	sa.computeLCP()
	return sa
}

// computeLCP 使用 Kasai 算法在 O(N) 时间内计算 LCP 数组。
func (sa *SuffixArray) computeLCP() {
	n := len(sa.text)
	k := 0
	for i := range n {
		if sa.rank[i] == 0 {
			sa.height[0] = 0
			continue
		}
		if k > 0 {
			k--
		}
		j := sa.sa[sa.rank[i]-1]
		for i+k < n && j+k < n && sa.text[i+k] == sa.text[j+k] {
			k++
		}
		sa.height[sa.rank[i]] = k
	}
}

// LongestRepeatedSubstring 寻找最长重复子串 (真实应用场景)。
func (sa *SuffixArray) LongestRepeatedSubstring() string {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	maxLCP := 0
	idx := -1
	for i, h := range sa.height {
		if h > maxLCP {
			maxLCP = h
			idx = i
		}
	}

	if idx == -1 {
		return ""
	}
	return sa.text[sa.sa[idx] : sa.sa[idx]+maxLCP]
}

func (sa *SuffixArray) build() {
	n := len(sa.text)
	m := 256 // 初始字符集大小。
	if n > m {
		m = n
	}
	if m < 256 {
		m = 256
	}

	x := make([]int, n)
	y := make([]int, n)
	c := make([]int, m)

	// 初始排序 (k=0)。
	for i := 0; i < n; i++ {
		x[i] = int(sa.text[i])
		c[x[i]]++
	}
	for i := 1; i < m; i++ {
		c[i] += c[i-1]
	}
	for i := n - 1; i >= 0; i-- {
		sa.sa[c[x[i]]-1] = i
		c[x[i]]--
	}

	// 倍增排序。
	for k := 1; k < n; k <<= 1 {
		p := 0
		// 1. 处理第二关键字。
		for i := n - k; i < n; i++ {
			y[p] = i
			p++
		}
		for i := 0; i < n; i++ {
			if sa.sa[i] >= k {
				y[p] = sa.sa[i] - k
				p++
			}
		}

		// 2. 基数排序处理第一关键字。
		for i := 0; i < m; i++ {
			c[i] = 0
		}
		for i := 0; i < n; i++ {
			c[x[y[i]]]++
		}
		for i := 1; i < m; i++ {
			c[i] += c[i-1]
		}
		for i := n - 1; i >= 0; i-- {
			sa.sa[c[x[y[i]]]-1] = y[i]
			c[x[y[i]]]--
		}

		// 3. 更新排名数组 x，并存入 y 暂存旧排名。
		x, y = y, x
		p = 1
		x[sa.sa[0]] = 0
		for i := 1; i < n; i++ {
			if sa.isEqual(y, sa.sa[i-1], sa.sa[i], k) {
				x[sa.sa[i]] = p - 1
			} else {
				x[sa.sa[i]] = p
				p++
			}
		}
		if p >= n {
			break
		}
		m = p // 更新字符集大小为当前排名总数。
	}

	// 填充最终排名。
	copy(sa.rank, x)
}

func (sa *SuffixArray) isEqual(rank []int, i, j, k int) bool {
	n := len(sa.text)
	if rank[i] != rank[j] {
		return false
	}
	ri, rj := -1, -1
	if i+k < n {
		ri = rank[i+k]
	}
	if j+k < n {
		rj = rank[j+k]
	}
	return ri == rj
}

// Search 在原始文本中搜索模式 (pattern) 的所有出现位置。
// 优化：避免切片产生的分配，直接进行字节比较。
func (sa *SuffixArray) Search(pattern string) []int {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	n := len(sa.text)
	m := len(pattern)
	if m == 0 {
		return nil
	}

	// 1. 寻找左边界。
	l, r := 0, n-1
	first := -1
	for l <= r {
		mid := l + (r-l)/2
		cmp := sa.compare(sa.sa[mid], pattern)
		if cmp >= 0 {
			if cmp == 0 {
				first = mid
			}
			r = mid - 1
		} else {
			l = mid + 1
		}
	}

	if first == -1 {
		return nil
	}

	// 2. 寻找右边界。
	l, r = first, n-1
	last := first
	for l <= r {
		mid := l + (r-l)/2
		cmp := sa.compare(sa.sa[mid], pattern)
		if cmp == 0 {
			last = mid
			l = mid + 1
		} else if cmp > 0 {
			r = mid - 1
		} else {
			l = mid + 1
		}
	}

	results := make([]int, 0, last-first+1)
	for i := first; i <= last; i++ {
		results = append(results, sa.sa[i])
	}
	return results
}

// compare 比较后缀与模式串，不产生分配。
func (sa *SuffixArray) compare(start int, pattern string) int {
	n := len(sa.text)
	m := len(pattern)
	for i := 0; i < m; i++ {
		if start+i >= n {
			return -1 // 后缀更短。
		}
		if sa.text[start+i] < pattern[i] {
			return -1
		}
		if sa.text[start+i] > pattern[i] {
			return 1
		}
	}
	return 0 // 前缀匹配成功。
}

// Count 统计模式串出现的次数。
func (sa *SuffixArray) Count(pattern string) int {
	pos := sa.Search(pattern)
	return len(pos)
}
