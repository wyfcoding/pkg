// Package validator 提供了通用的数据合法性校验工具函数。
package validator

import (
	"regexp"
	"strings"
)

var (
	phoneRegex    = regexp.MustCompile(`^1[3-9]\d{9}$`)                                    // 中国大陆手机号格式。
	emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`) // 邮箱校验。
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{4,20}$`)                             // 用户名校验。
	passwordRegex = regexp.MustCompile(`^[A-Za-z\d@$!%*#?&]{8,}$`)                         // 基础密码校验。
)

// IsValidPhone 校验字符串是否为合法的中国大陆手机号。
func IsValidPhone(phone string) bool {
	return phoneRegex.MatchString(phone)
}

// IsValidEmail 校验字符串是否符合标准电子邮件格式。
func IsValidEmail(email string) bool {
	return emailRegex.MatchString(email)
}

// IsValidUsername 校验用户名是否符合安全长度和字符限制。
func IsValidUsername(username string) bool {
	return usernameRegex.MatchString(username)
}

// IsValidPassword 执行增强的密码强度校验。
func IsValidPassword(password string) bool {
	if !passwordRegex.MatchString(password) {
		return false
	}

	hasLetter := false
	hasDigit := false

	for _, char := range password {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') {
			hasLetter = true
		}

		if char >= '0' && char <= '9' {
			hasDigit = true
		}

		if hasLetter && hasDigit {
			return true
		}
	}

	return false
}

// IsEmpty 判断去空格后的字符串是否为空。
func IsEmpty(val string) bool {
	return strings.TrimSpace(val) == ""
}

// IsValidLength 校验字符串是否在指定长度闭区间内。
func IsValidLength(val string, minLen, maxLen int) bool {
	length := len([]rune(val))

	return length >= minLen && length <= maxLen
}

// IsPositive 判断数字是否为正数。
func IsPositive(num int64) bool {
	return num > 0
}

// IsNonNegative 判断数字是否为非负数。
func IsNonNegative(num int64) bool {
	return num >= 0
}

// IsInRange 校验数字是否在指定闭区间内。
func IsInRange(num, minVal, maxVal int64) bool {
	return num >= minVal && num <= maxVal
}
