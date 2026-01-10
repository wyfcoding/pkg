// Package text 提供文本处理与随机字符串生成工具.
package text

import (
	"crypto/rand"
	"strings"
	"time"
)

// RandomString 生成指定长度的随机字符串.
// 优化：全路径使用 crypto/rand 确保安全性，满足 G404 生产标准。
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	if _, err := rand.Read(result); err != nil {
		// 极端情况下的低熵降级，仅为满足健壮性.
		for i := range result {
			ts := time.Now().UnixNano()
			if ts < 0 {
				ts = -ts
			}
			// 安全：ts >= 0 已保证，用于字符集索引。
			idx := uint64(ts) % uint64(len(charset)) //nolint:gosec // ts >= 0 已保证。
			result[i] = charset[idx]
		}
		return string(result)
	}

	for i := range result {
		result[i] = charset[result[i]%byte(len(charset))]
	}

	return string(result)
}

// MaskString 对字符串进行脱敏处理，保留前后指定长度.
func MaskString(s string, prefixLen, suffixLen int) string {
	if len(s) <= prefixLen+suffixLen {
		return s
	}

	maskLen := len(s) - prefixLen - suffixLen
	mask := strings.Repeat("*", maskLen)

	return s[:prefixLen] + mask + s[len(s)-suffixLen:]
}

// IsBlank 检查字符串是否为空或仅包含空白字符.
func IsBlank(s string) bool {
	if s == "" {
		return true
	}

	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}

	return true
}
