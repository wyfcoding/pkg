package security

import (
	"golang.org/x/crypto/bcrypt"
)

// HashPassword 生成密码的 bcrypt 哈希值。
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword 检查提供的密码是否与哈希值匹配。
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
