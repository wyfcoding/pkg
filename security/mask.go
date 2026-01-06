// Package security 提供了安全相关的通用功能。
// 此文件实现了常用的敏感数据脱敏（Masking）逻辑，用于在日志或界面中隐藏部分敏感信息。
package security

import (
	"strings"
)

// MaskPhone 针对中国大陆手机号进行脱敏处理。
// 规则：保留前 3 位和后 4 位，中间用 4 个星号代替 (例如: 138****5678)。
func MaskPhone(phone string) string {
	if len(phone) < 11 {
		return phone
	}
	return phone[:3] + "****" + phone[7:]
}

// MaskEmail 针对电子邮件地址进行脱敏处理。
// 规则：保留前 2 位字符，用户名剩余部分用 3 个星号代替，域名完整保留。
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

// MaskIDCard 针对二代身份证号（15 或 18 位）进行脱敏处理。
// 规则：保留前 3 位和后 4 位，中间部分根据长度填充星号。
func MaskIDCard(idCard string) string {
	length := len(idCard)
	if length < 15 {
		return idCard
	}
	return idCard[:3] + strings.Repeat("*", length-7) + idCard[length-4:]
}

// MaskBankCard 针对银行卡号进行脱敏处理。
// 规则：保留前 4 位和后 4 位，中间部分填充星号。
func MaskBankCard(cardNo string) string {
	length := len(cardNo)
	if length < 12 {
		return cardNo
	}
	return cardNo[:4] + strings.Repeat("*", length-8) + cardNo[length-4:]
}

// MaskRealName 针对中文真实姓名进行脱敏处理。
// 规则：
// 1. 2 个字姓名：掩码第 2 位 (张三 -> 张*)。
// 2. 3 个字及以上：保留第 1 位，后续全部掩码 (欧阳锋 -> 欧**)。
func MaskRealName(name string) string {
	runes := []rune(name)
	length := len(runes)
	if length <= 1 {
		return name
	}
	if length == 2 {
		return string(runes[0]) + "*"
	}
	return string(runes[0]) + strings.Repeat("*", length-1)
}

// MaskString 提供通用的字符串掩码逻辑。
// 参数：s 原始字符串，prefixLen 前部保留长度，suffixLen 后部保留长度。
func MaskString(s string, prefixLen, suffixLen int) string {
	runes := []rune(s)
	length := len(runes)
	if length <= prefixLen+suffixLen {
		return s
	}
	return string(runes[:prefixLen]) + "****" + string(runes[length-suffixLen:])
}
