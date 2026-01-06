// Package validator 提供了通用的数据合法性校验工具函数，涵盖常用的正则表达式与逻辑判定。
package validator

import (
	"regexp"
	"strings"
)

// 定义全项目共用的业务级正则表达式。
var (
	phoneRegex    = regexp.MustCompile(`^1[3-9]\d{9}$`)                                    // 中国大陆手机号格式
	emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`) // 工业级邮箱校验规则
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{4,20}$`)                             // 用户名：4-20位，仅限字母数字下划线
	passwordRegex = regexp.MustCompile(`^[A-Za-z\d@$!%*#?&]{8,}$`)                         // 基础密码：至少8位
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
// 除了满足正则表达式（最小长度）外，强制要求同时包含字母和数字。
func IsValidPassword(password string) bool {
	if !passwordRegex.MatchString(password) {
		return false
	}
	// 针对 Go 正则表达式特性的手动逻辑补充
	hasLetter := false
	hasDigit := false
	for _, c := range password {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			hasLetter = true
		}
		if c >= '0' && c <= '9' {
			hasDigit = true
		}
		if hasLetter && hasDigit {
			return true
		}
	}
	return false
}

// IsEmpty 判断去空格后的字符串是否为空。
func IsEmpty(s string) bool {
	return strings.TrimSpace(s) == ""
}

// IsValidLength 校验字符串（以 rune 为单位，支持 Unicode）是否在指定长度闭区间内。
func IsValidLength(s string, min, max int) bool {
	length := len([]rune(s))
	return length >= min && length <= max
}

// ... (数字校验函数保持不变) ...
func IsPositive(n int64) bool {
	return n > 0
}

func IsNonNegative(n int64) bool {
	return n >= 0
}

func IsInRange(n, min, max int64) bool {
	return n >= min && n <= max
}
