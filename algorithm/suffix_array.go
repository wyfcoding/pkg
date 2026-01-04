package algorithm

import (
	"sort"
	"sync"
)

// SuffixArray 结构体实现了后缀数组。
// 后缀数组是一个字符串所有后缀的排序数组，它在字符串匹配、模式查找等领域有广泛应用。
// 相较于后缀树，后缀数组在空间效率上通常更优。
type SuffixArray struct {
	text   string       // 原始字符串。
	sa     []int        // 后缀数组
	rank   []int        // 排名数组
	height []int        // LCP 数组：height[i] 表示 sa[i] 和 sa[i-1] 后缀的最长公共前缀
	mu     sync.RWMutex // 读写锁
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

// ... (build() logic remains same)

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
	// 初始化：sa[i] = i，rank[i] = text[i] 的ASCII值。
	// 此时，sa 存储的是所有后缀的起始索引，rank 存储的是长度为1的后缀的排名。
	for i := range n {
		sa.sa[i] = i
		sa.rank[i] = int(sa.text[i])
	}

	// 倍增法迭代：每次迭代将比较的后缀长度加倍 (k -> 2k)。
	for k := 1; k < n; k *= 2 {
		// 根据当前长度 k 的排名和长度 k 的下一段后缀的排名，对后缀进行排序。
		sort.Slice(sa.sa, func(i, j int) bool {
			a, b := sa.sa[i], sa.sa[j]
			// 首先比较当前长度 k 的后缀排名。
			if sa.rank[a] != sa.rank[b] {
				return sa.rank[a] < sa.rank[b]
			}
			// 如果当前长度 k 的后缀排名相同，则比较长度为 k 的下一段后缀的排名。
			ra := 0
			rb := 0
			// 确保索引不越界。
			if a+k < n {
				ra = sa.rank[a+k]
			}
			if b+k < n {
				rb = sa.rank[b+k]
			}
			return ra < rb
		})

		// 根据新的排序结果更新 rank 数组。
		newRank := make([]int, n)
		newRank[sa.sa[0]] = 0 // 排名第一的后缀的排名为0。
		for i := 1; i < n; i++ {
			newRank[sa.sa[i]] = newRank[sa.sa[i-1]] // 默认与前一个后缀排名相同。
			a, b := sa.sa[i-1], sa.sa[i]
			// 如果当前后缀与前一个后缀在长度 k 上不相同，则排名增加。
			if sa.rank[a] != sa.rank[b] {
				newRank[sa.sa[i]]++
			} else {
				// 如果长度 k 上相同，则比较长度 k 的下一段后缀。
				ra, rb := 0, 0
				if a+k < n {
					ra = sa.rank[a+k]
				}
				if b+k < n {
					rb = sa.rank[b+k]
				}
				if ra != rb {
					newRank[sa.sa[i]]++
				}
			}
		}
		sa.rank = newRank // 更新 rank 数组。
	}
}

// Search 在原始文本中搜索模式 (pattern) 的所有出现位置。
// 使用二分查找利用后缀数组的有序性。
func (sa *SuffixArray) Search(pattern string) []int {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	n := len(sa.text)
	m := len(pattern)
	if m == 0 {
		return nil
	}

	// 1. 寻找左边界 (第一个 >= pattern 的位置)
	l, r := 0, n-1
	first := -1
	for l <= r {
		mid := l + (r-l)/2
		suffix := sa.getSuffix(sa.sa[mid], m)
		if suffix >= pattern {
			first = mid
			r = mid - 1
		} else {
			l = mid + 1
		}
	}

	if first == -1 || sa.getSuffix(sa.sa[first], m) != pattern {
		return nil
	}

	// 2. 寻找右边界 (最后一个 == pattern 的位置)
	l, r = first, n-1
	last := first
	for l <= r {
		mid := l + (r-l)/2
		suffix := sa.getSuffix(sa.sa[mid], m)
		if suffix == pattern {
			last = mid
			l = mid + 1
		} else if suffix > pattern {
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

// getSuffix 获取从 start 开始长度为 length 的后缀（处理越界）
func (sa *SuffixArray) getSuffix(start, length int) string {
	end := min(start+length, len(sa.text))
	return sa.text[start:end]
}

// Count 统计模式串出现的次数
func (sa *SuffixArray) Count(pattern string) int {
	pos := sa.Search(pattern)
	return len(pos)
}
