// Package security 提供了安全相关的通用功能，包括密码哈希、敏感数据脱敏及对称加密等。
package security

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword 生成给定明文密码的 bcrypt 加密哈希值。
// 采用默认的计算强度 (DefaultCost)，确保在安全性和性能之间取得平衡。
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword 验证提供的明文密码是否与存储的 bcrypt 哈希值匹配。
// 返回 true 表示验证通过，false 表示密码错误。
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
