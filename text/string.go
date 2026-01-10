// Package text 提供了字符串处理、随机生成等基础工具。
package text

import (
	"crypto/md5" //nolint:gosec // 仅用于生成非安全哈希（如文件名）。
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	randv2 "math/rand/v2"
	"strings"
	"time"
)

// RandomString 生成指定长度的随机字符串（由数字和字母组成）。
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)

	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		// 极低概率失败，退化为时间戳。
		binary.LittleEndian.PutUint64(seed[:], uint64(time.Now().UnixNano()))
	}

	randomSrc := randv2.New(randv2.NewPCG(binary.LittleEndian.Uint64(seed[:]), 0))

	for i := range result {
		result[i] = charset[randomSrc.IntN(len(charset))]
	}

	return string(result)
}

// MD5 计算字符串的 MD5 哈希值（返回 32 位十六进制字符串）。
// 注意：MD5 已不再适用于密码学安全场景，仅用于普通校验。
func MD5(text string) string {
	hash := md5.New() //nolint:gosec
	hash.Write([]byte(text))

	return hex.EncodeToString(hash.Sum(nil))
}

// Mask 字符串脱敏处理（保留前 prefixLen 位和后 suffixLen 位，中间用 * 代替）。
func Mask(s string, prefixLen, suffixLen int) string {
	if len(s) <= prefixLen+suffixLen {
		return s
	}

	return s[:prefixLen] + "****" + s[len(s)-suffixLen:]
}

// IsAnyEmpty 检查是否有任意一个字符串为空。
func IsAnyEmpty(ss ...string) bool {
	for _, s := range ss {
		if strings.TrimSpace(s) == "" {
			return true
		}
	}

	return false
}