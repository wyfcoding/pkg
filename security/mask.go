package security

import (
	"strings"
)

// MaskPhone 手机号脱敏 (13812345678 -> 138****5678)
func MaskPhone(phone string) string {
	if len(phone) < 11 {
		return phone
	}
	return phone[:3] + "****" + phone[7:]
}

// MaskEmail 邮箱脱敏 (example@gmail.com -> ex***@gmail.com)
func MaskEmail(email string) string {
	if !strings.Contains(email, "@") {
		return email
	}
	parts := strings.Split(email, "@")
	if len(parts[0]) < 3 {
		return "***@" + parts[1]
	}
	return parts[0][:2] + "***@" + parts[1]
}

// MaskIDCard 身份证脱敏 (110101199001011234 -> 110**********1234)
func MaskIDCard(idCard string) string {
	length := len(idCard)
	if length < 15 {
		return idCard
	}
	return idCard[:3] + strings.Repeat("*", length-7) + idCard[length-4:]
}

// MaskBankCard 银行卡脱敏 (6222021234567890 -> 6222**********7890)
func MaskBankCard(cardNo string) string {
	length := len(cardNo)
	if length < 12 {
		return cardNo
	}
	return cardNo[:4] + strings.Repeat("*", length-8) + cardNo[length-4:]
}

// MaskRealName 真实姓名脱敏 (张三 -> 张*, 欧阳锋 -> 欧阳**)
func MaskRealName(name string) string {
	runes := []rune(name)
	length := len(runes)
	if length <= 1 {
		return name
	}
	if length == 2 {
		return string(runes[0]) + "*"
	}
	// 保留第一位，其余掩码
	return string(runes[0]) + strings.Repeat("*", length-1)
}

// MaskString 通用掩码函数 (保留前后固定长度)
func MaskString(s string, prefixLen, suffixLen int) string {
	runes := []rune(s)
	length := len(runes)
	if length <= prefixLen+suffixLen {
		return s
	}
	return string(runes[:prefixLen]) + "****" + string(runes[length-suffixLen:])
}
