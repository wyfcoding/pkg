package algorithm

import (
	"math/bits"
	"regexp"
	"strings"
)

var wordRegex = regexp.MustCompile(`\w+`)

// SimHash 局部敏感哈希算法实现。
// 适用于：文本去重、相似文档发现。
type SimHash struct {
	hashBits int // 哈希位数，通常为 64。
}

func NewSimHash() *SimHash {
	return &SimHash{hashBits: 64}
}

// Calculate 计算文本的 SimHash 值。
func (s *SimHash) Calculate(text string) uint64 {
	// 1. 分词与清洗。
	// 使用正则提取所有单词，并转为小写以忽略大小写差异。
	words := wordRegex.FindAllString(strings.ToLower(text), -1)
	if len(words) == 0 {
		return 0
	}

	// 2. 构造 N-grams (Bi-grams) 增强上下文特征。
	tokens := make([]string, 0, len(words)*2)
	tokens = append(tokens, words...)
	for i := 0; i < len(words)-1; i++ {
		tokens = append(tokens, words[i]+"_"+words[i+1])
	}

	// 3. 初始化加权向量。
	weights := make([]int, s.hashBits)

	for _, token := range tokens {
		// 计算词元（包含单词和双词组）的哈希值。
		h := fnvHash(token)

		// 4. 加权计算。
		for i := 0; i < s.hashBits; i++ {
			if (h >> uint(i) & 1) == 1 {
				weights[i]++
			} else {
				weights[i]--
			}
		}
	}

	// 5. 降维 (转换回 0/1 位串)。
	var result uint64
	for i := 0; i < s.hashBits; i++ {
		if weights[i] > 0 {
			result |= (1 << uint(i))
		}
	}

	return result
}

// HammingDistance 计算两个 SimHash 的海明距离。
// 距离越小，文本越相似。通常距离 <= 3 被认为是高度相似。
func (s *SimHash) HammingDistance(h1, h2 uint64) int {
	return bits.OnesCount64(h1 ^ h2)
}

// IsSimilar 判断两个文本是否相似。
func (s *SimHash) IsSimilar(text1, text2 string, threshold int) bool {
	h1 := s.Calculate(text1)
	h2 := s.Calculate(text2)
	return s.HammingDistance(h1, h2) <= threshold
}

func fnvHash(s string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	var h uint64 = offset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}
