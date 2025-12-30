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

// EncryptAES 使用AES-GCM模式加密给定的明文。
func EncryptAES(key []byte, plaintext string) (string, error) {
	// 校验密钥长度
	if l := len(key); l != 16 && l != 24 && l != 32 {
		return "", errors.New("invalid key length: must be 16, 24, or 32 bytes")
	}

	// 1. 基于给定的密钥创建一个新的AES密码块
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// 2. 基于密码块创建一个GCM（伽罗瓦/计数器模式）实例
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// 3. 创建一个nonce（只使用一次的随机数）
	// GCM的安全性严重依赖于nonce的唯一性，对于相同的密钥，绝不能重复使用nonce。
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// 4. 执行加密
	// Seal函数将nonce、明文、以及可选的附加认证数据(AAD)加密成密文。
	// 这里我们将nonce作为密文的前缀，方便解密时提取。
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// 5. 将二进制密文编码为Base64字符串，使其更易于传输
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAES 使用AES-GCM模式解密给定的密文。
// 它期望密文是经过Base64编码的，并且nonce作为密文的初始部分。
func DecryptAES(key []byte, cryptoText string) (string, error) {
	// 1. 将Base64编码的密文解码为二进制数据
	ciphertext, err := base64.StdEncoding.DecodeString(cryptoText)
	if err != nil {
		return "", err
	}

	// 2. 创建AES密码块和GCM实例
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// 3. 从密文中分离nonce和实际的密文部分
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// 4. 执行解密
	// Open函数会验证密文的完整性和真实性，如果校验失败则返回错误。
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// 解密失败（例如，密钥错误或密文被篡改）
		return "", err
	}

	// 5. 将解密后的字节数组转换为字符串
	return string(plaintext), nil
}
