package structures

import (
	"math/bits"
	"regexp"
	"strings"

	algomath "github.com/wyfcoding/pkg/algorithm/math"
)

var wordRegex = regexp.MustCompile(`\w+`)

const (
	defaultHashBits = 64
)

// SimHash 局部敏感哈希算法实现.
type SimHash struct {
	hashBits int
}

// NewSimHash 创建一个新的 SimHash 实例.
func NewSimHash() *SimHash {
	return &SimHash{hashBits: defaultHashBits}
}

// Calculate 计算文本的 SimHash 值.
func (s *SimHash) Calculate(text string) uint64 {
	words := wordRegex.FindAllString(strings.ToLower(text), -1)
	if len(words) == 0 {
		return 0
	}

	tokens := make([]string, 0, len(words)*2)
	tokens = append(tokens, words...)
	for i := range words[:len(words)-1] {
		tokens = append(tokens, words[i]+"_"+words[i+1])
	}

	weights := make([]int, s.hashBits)

	for _, token := range tokens {
		h := getFNVHash(token)

		for i := range s.hashBits {
			// 安全：i 范围 [0, 63]，位移量安全。
			if (h & (uint64(1) << uint32(i&0x3F))) != 0 { //nolint:gosec // i 范围 [0, 63]。
				weights[i]++
			} else {
				weights[i]--
			}
		}
	}

	var result uint64
	for i := range s.hashBits {
		if weights[i] > 0 {
			// 安全：i 范围 [0, 63]，位移量安全。
			result |= (1 << uint32(i&0x3F)) //nolint:gosec // i 范围 [0, 63]。
		}
	}

	return result
}

// HammingDistance 计算两个 SimHash 的海明距离.
func (s *SimHash) HammingDistance(h1, h2 uint64) int {
	return bits.OnesCount64(h1 ^ h2)
}

// IsSimilar 判断两个文本是否相似.
func (s *SimHash) IsSimilar(text1, text2 string, threshold int) bool {
	hash1 := s.Calculate(text1)
	hash2 := s.Calculate(text2)

	return s.HammingDistance(hash1, hash2) <= threshold
}

func getFNVHash(s string) uint64 {
	var h uint64 = algomath.FnvOffset64
	for i := range s {
		h ^= uint64(s[i])
		h *= algomath.FnvPrime64
	}

	return h
}
