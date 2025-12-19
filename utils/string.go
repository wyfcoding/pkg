package utils

import (
	"crypto/md5"   // 导入MD5哈希算法库。
	"encoding/hex" // 导入hex编码解码库。
	"math/rand"    // 导入随机数生成库。
	"strings"      // 导入字符串操作库。
	"time"         // 导入时间操作库。
)

// RandomString 生成指定长度的随机字符串。
// 随机字符串由大小写字母和数字组成。
// length: 待生成的随机字符串的长度。
// 返回生成的随机字符串。
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	// 使用当前时间戳作为随机数种子，确保每次生成的随机数序列不同。
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[r.Intn(len(charset))] // 从字符集中随机选择字符。
	}
	return string(b)
}

// MD5 计算给定字符串的MD5哈希值。
// MD5是一种广泛使用的密码散列函数，可以生成一个128位的散列值（通常表示为32位十六进制数）。
// s: 待计算哈希的字符串。
// 返回字符串的MD5哈希值（小写十六进制字符串）。
func MD5(s string) string {
	h := md5.New()                        // 创建一个新的MD5哈希实例。
	h.Write([]byte(s))                    // 将字符串数据写入哈希实例。
	return hex.EncodeToString(h.Sum(nil)) // 计算哈希值并编码为十六进制字符串。
}

// TruncateString 截断字符串到指定的最大长度，并在末尾添加 "..."。
// 如果字符串的长度小于或等于maxLen，则不进行截断。
// s: 待截断的字符串。
// maxLen: 字符串的最大保留长度。
// 返回截断后的字符串。
func TruncateString(s string, maxLen int) string {
	runes := []rune(s) // 将字符串转换为rune切片，以正确处理Unicode字符。
	if len(runes) <= maxLen {
		return s // 如果字符串长度未超过最大长度，则直接返回原字符串。
	}
	// 截断字符串并在末尾添加 "..."。
	return string(runes[:maxLen]) + "..."
}

// Contains 检查字符串 s 是否包含任何一个给定的子串。
// s: 待检查的字符串。
// substrs: 子串列表。
// 返回：如果 s 包含任何一个子串，则返回 true；否则返回 false。
func Contains(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

// IsBlank 检查字符串是否为空或只包含空白字符。
// s: 待检查的字符串。
// 返回：如果字符串为空或只包含空白字符，则返回 true；否则返回 false。
func IsBlank(s string) bool {
	return strings.TrimSpace(s) == "" // 去除字符串两端的空白字符后，检查是否为空。
}
