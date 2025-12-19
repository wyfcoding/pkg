package utils

import "fmt"

// FenToYuan 将金额从“分”转换为“元”。
// 在处理货币时，通常使用整数类型存储“分”以避免浮点数精度问题，
// 在展示给用户时再转换为“元”。
// fen: 以分为单位的金额。
// 返回以元为单位的浮点数金额。
func FenToYuan(fen int64) float64 {
	return float64(fen) / 100.0
}

// YuanToFen 将金额从“元”转换为“分”。
// yuan: 以元为单位的浮点数金额。
// 返回以分为单位的整数金额。
func YuanToFen(yuan float64) int64 {
	// 乘以100并转换为int64，可能会丢失浮点数精度，
	// 实际生产中可能需要更严谨的舍入或处理方式。
	return int64(yuan * 100)
}

// FormatMoney 将以“分”为单位的金额格式化为带有两位小数的字符串（以元为单位）。
// fen: 以分为单位的金额。
// 返回格式化后的字符串，例如 "12.34"。
func FormatMoney(fen int64) string {
	yuan := FenToYuan(fen)
	return fmt.Sprintf("%.2f", yuan) // 格式化为两位小数的浮点数。
}
