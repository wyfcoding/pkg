// Package text 提供了字符串处理、随机生成等基础工具.
package text

import (
	"crypto/md5" // 仅用于生成非安全哈希.
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	randv2 "math/rand/v2"
	"strings"
	"time"
)

// RandomString 生成指定长度的随机字符串.
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)

	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		binary.LittleEndian.PutUint64(seed[:], uint64(time.Now().UnixNano()))
	}

	randomSrc := randv2.New(randv2.NewPCG(binary.LittleEndian.Uint64(seed[:]), 0))

	for i := range result {
		result[i] = charset[randomSrc.IntN(len(charset))]
	}

	return string(result)
}

// MD5 计算字符串的 MD5 哈希值.
func MD5(text string) string {
	//nolint:gosec // 经过审计，此处仅用于普通哈希校验，不涉及安全场景.
	hash := md5.New()
	hash.Write([]byte(text))

	return hex.EncodeToString(hash.Sum(nil))
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
