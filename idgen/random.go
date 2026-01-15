// Package idgen 提供了分布式唯一 ID 生成器的实现.
package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenShortID 生成一个短的随机 ID (类似 nanoid).
func GenShortID(length int) string {
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Errorf("failed to generate random bytes: %w", err))
	}
	for i, b := range bytes {
		bytes[i] = charset[b%byte(len(charset))]
	}
	return string(bytes)
}

// GenerateRandomID 生成指定长度的随机十六进制字符串 ID.
func GenerateRandomID(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
