// Package security 提供了加密、哈希等安全相关的函数。
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// GenerateRandomKey 生成加密安全的随机密钥 (32字节，适用于 AES-256)
func GenerateRandomKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// NewNonce 创建一个新的 nonce（只使用一次的随机数）。
// GCM 的安全性严重依赖于 nonce 的唯一性，对于相同的密钥，绝不能重复使用 nonce。
func NewNonce(gcm cipher.AEAD) ([]byte, error) {
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return nonce, nil
}

// EncryptAES 使用 AES-GCM 模式对给定的明文进行认证加密。
// 流程：生成随机 Nonce -> 执行加密 -> 将 Nonce 作为密文前缀 -> Base64 编码。
func EncryptAES(key []byte, plaintext string) (string, error) {
	// ... (校验密钥长度保持不变) ...
	if l := len(key); l != 16 && l != 24 && l != 32 {
		return "", errors.New("invalid key length: must be 16, 24, or 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create gcm instance: %w", err)
	}

	nonce, err := NewNonce(gcm)
	if err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// 执行 Seal 加密操作，将结果拼接在 nonce 之后
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAES 使用 AES-GCM 模式解密 Base64 编码的加密字符串。
// 流程：Base64 解码 -> 提取 Nonce -> 执行认证解密。
func DecryptAES(key []byte, cryptoText string) (string, error) {
	// 1. 执行 Base64 解码获取原始密文
	ciphertext, err := base64.StdEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create gcm instance: %w", err)
	}

	// 从数据头部截取预定长度的 Nonce
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short to contain nonce")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// 执行 Open 解密并验证数据的真实性（防止篡改）
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt or verify data: %w", err)
	}

	return string(plaintext), nil
}
