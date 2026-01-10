// Package text 提供了字符串处理、随机生成等基础工具.
package text

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	randv2 "math/rand/v2"
	"strings"
	"time"
)

// RandomString 生成指定长度的随机字符串.
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	if _, err := rand.Read(result); err != nil {
		// 降级使用 time 相关作为熵（非理想，但作为兜底）.
		ts := time.Now().UnixNano()
		randomSrc := randv2.New(randv2.NewPCG(uint64(ts), 0))
		for i := range result {
			result[i] = charset[randomSrc.IntN(len(charset))]
		}
		return string(result)
	}

	for i := range result {
		result[i] = charset[result[i]%byte(len(charset))]
	}

	return string(result)
}

// SHA256 计算字符串的 SHA256 哈希值.
func SHA256(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

// Mask 字符串脱敏处理.
func Mask(s string, prefixLen, suffixLen int) string {
	if len(s) <= prefixLen+suffixLen {
		return s
	}

	return s[:prefixLen] + "****" + s[len(s)-suffixLen:]
}

// IsAnyEmpty 检查是否有任意一个字符串为空.
func IsAnyEmpty(ss ...string) bool {
	for _, s := range ss {
		if strings.TrimSpace(s) == "" {
			return true
		}
	}

	return false
}
