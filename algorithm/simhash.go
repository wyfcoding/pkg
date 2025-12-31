package algorithm

import (
	"hash/fnv"
	"strings"
)

// SimHash 局部敏感哈希算法实现
// 适用于：文本去重、相似文档发现。
type SimHash struct {
	hashBits int // 哈希位数，通常为 64
}

func NewSimHash() *SimHash {
	return &SimHash{hashBits: 64}
}

// Calculate 计算文本的 SimHash 值
func (s *SimHash) Calculate(text string) uint64 {
	// 1. 分词 (简化处理：按空格或简单的字符分割)
	words := strings.Fields(text)
	if len(words) == 0 {
		return 0
	}

	// 2. 初始化加权向量
	v := make([]int, s.hashBits)

	for _, word := range words {
		// 计算单词的传统哈希值
		h := fnvHash(word)
		
		// 3. 加权
		// 如果对应位为1，权重+1；为0，权重-1
		for i := 0; i < s.hashBits; i++ {
			if (h >> uint(i) & 1) == 1 {
				v[i]++
			} else {
				v[i]--
			}
		}
	}

	// 4. 降维 (降维回 0/1)
	var result uint64
	for i := 0; i < s.hashBits; i++ {
		if v[i] > 0 {
			result |= (1 << uint(i))
		}
	}

	return result
}

// HammingDistance 计算两个 SimHash 的海明距离
// 距离越小，文本越相似。通常距离 <= 3 被认为是高度相似。
func (s *SimHash) HammingDistance(h1, h2 uint64) int {
	x := h1 ^ h2
	count := 0
	for x > 0 {
		count++
		x &= x - 1
	}
	return count
}

// IsSimilar 判断两个文本是否相似
func (s *SimHash) IsSimilar(text1, text2 string, threshold int) bool {
	h1 := s.Calculate(text1)
	h2 := s.Calculate(text2)
	return s.HammingDistance(h1, h2) <= threshold
}

func fnvHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
