package idgen

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateRandomID 生成指定长度的随机十六进制字符串 ID.
func GenerateRandomID(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
