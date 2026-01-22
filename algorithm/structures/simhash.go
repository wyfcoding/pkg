package structures

import (
	"math/bits"
	"regexp"
	"strings"

	algomath "github.com/wyfcoding/pkg/algorithm/math"
)

var defaultSimHashWordRegex = regexp.MustCompile(`\w+`)

const (
	defaultHashBits = 64
)

// SimHash 局部敏感哈希算法实现.
type SimHash struct {
	hashBits uint
}

// NewSimHash 创建一个新的 SimHash 实例.
func NewSimHash() *SimHash {
	return &SimHash{hashBits: defaultHashBits}
}

// Tokenizer 将文本分割为词元及其权重.
type Tokenizer func(text string) map[string]int

// DefaultTokenizer 默认的分词器实现 (1-gram + 2-gram).
func DefaultTokenizer(text string) map[string]int {
	words := defaultSimHashWordRegex.FindAllString(strings.ToLower(text), -1)
	if len(words) == 0 {
		return nil
	}

	tokens := make(map[string]int)
	for _, w := range words {
		tokens[w]++
	}
	for i := range len(words) - 1 {
		tokens[words[i]+"_"+words[i+1]]++
	}
	return tokens
}

// Calculate 使用指定分词器计算文本的 SimHash 值.
func (s *SimHash) Calculate(text string, tokenizer Tokenizer) uint64 {
	tokens := tokenizer(text)
	if len(tokens) == 0 {
		return 0
	}

	weights := make([]int, s.hashBits)

	for token, weight := range tokens {
		h := getFNVHash(token)

		for i := range s.hashBits {
			if (h & (uint64(1) << i)) != 0 {
				weights[i] += weight
			} else {
				weights[i] -= weight
			}
		}
	}

	var result uint64
	for i := range s.hashBits {
		if weights[i] > 0 {
			result |= (uint64(1) << i)
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
	hash1 := s.Calculate(text1, DefaultTokenizer)
	hash2 := s.Calculate(text2, DefaultTokenizer)

	return s.HammingDistance(hash1, hash2) <= threshold
}

func getFNVHash(s string) uint64 {
	var h uint64 = algomath.FnvOffset64
	for i := range len(s) {
		h ^= uint64(s[i])
		h *= algomath.FnvPrime64
	}
	return h
}
