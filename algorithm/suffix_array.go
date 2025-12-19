package algorithm

import (
	"sort"
	"sync"
)

// SuffixArray 结构体实现了后缀数组。
// 后缀数组是一个字符串所有后缀的排序数组，它在字符串匹配、模式查找等领域有广泛应用。
// 相较于后缀树，后缀数组在空间效率上通常更优。
type SuffixArray struct {
	text string       // 原始字符串。
	sa   []int        // 后缀数组，存储排序后的后缀的起始索引。
	rank []int        // rank[i] 表示后缀 text[i:] 在所有后缀中的排名。
	mu   sync.RWMutex // 读写锁，用于保护后缀数组的并发访问。
}

// NewSuffixArray 创建并返回一个新的 SuffixArray 实例。
// text: 用于构建后缀数组的原始字符串。
func NewSuffixArray(text string) *SuffixArray {
	sa := &SuffixArray{
		text: text,
		sa:   make([]int, len(text)),
		rank: make([]int, len(text)),
	}
	sa.build() // 构建后缀数组。
	return sa
}

// build 构建后缀数组。
// 此处使用倍增法（Doubling Algorithm）来构建后缀数组，时间复杂度为 O(N log N)。
func (sa *SuffixArray) build() {
	n := len(sa.text)
	// 初始化：sa[i] = i，rank[i] = text[i] 的ASCII值。
	// 此时，sa 存储的是所有后缀的起始索引，rank 存储的是长度为1的后缀的排名。
	for i := 0; i < n; i++ {
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

// Search 在原始文本中搜索所有模式 (pattern) 的出现位置。
// 应用场景：商品搜索优化、日志分析等。
// pattern: 待搜索的模式字符串。
// 返回模式在原始文本中所有出现的起始索引列表。
func (sa *SuffixArray) Search(pattern string) []int {
	sa.mu.RLock()         // 搜索操作只需要读锁。
	defer sa.mu.RUnlock() // 确保函数退出时解锁。

	results := make([]int, 0)
	// 遍历排序后的后缀数组。
	// 这是一个简单的线性扫描，更高效的搜索通常结合二分查找。
	for _, pos := range sa.sa {
		// 检查当前后缀是否足够长以包含模式。
		if pos+len(pattern) <= len(sa.text) {
			// 如果当前后缀以模式开头，则记录该位置。
			if sa.text[pos:pos+len(pattern)] == pattern {
				results = append(results, pos)
			}
		}
	}
	return results
}
