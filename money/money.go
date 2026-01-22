// Package money 提供了基于 shopspring/decimal 的高精度货币计算与处理能力.
package money

import (
	"github.com/shopspring/decimal"
)

// Money 封装了高精度的金额处理.
type Money struct {
	value decimal.Decimal
}

// New 从 float64 创建 Money.
func New(val float64) Money {
	return Money{value: decimal.NewFromFloat(val)}
}

// NewFromInt 从 int64 (通常是分) 创建 Money.
func NewFromInt(val int64) Money {
	return Money{value: decimal.NewFromInt(val)}
}

// NewFromFen 从分为单位的整数创建 Money.
func NewFromFen(fen int64) Money {
	return Money{value: decimal.NewFromInt(fen).Div(decimal.NewFromInt(100))}
}

// NewFromString 从字符串解析金额.
func NewFromString(val string) (Money, error) {
	d, err := decimal.NewFromString(val)
	if err != nil {
		return Money{}, err
	}
	return Money{value: d}, nil
}

// ToFen 转换为以分为单位的整数 (四舍五入).
func (m Money) ToFen() int64 {
	return m.value.Mul(decimal.NewFromInt(100)).Round(0).IntPart()
}

// ToFloat 转换为 float64.
func (m Money) ToFloat() float64 {
	f, _ := m.value.Float64()
	return f
}

// String 返回格式化后的字符串 (默认 2 位小数).
func (m Money) String() string {
	return m.value.StringFixed(2)
}

// Add 加法.
func (m Money) Add(other Money) Money {
	return Money{value: m.value.Add(other.value)}
}

// Sub 减法.
func (m Money) Sub(other Money) Money {
	return Money{value: m.value.Sub(other.value)}
}

// Mul 乘法.
func (m Money) Mul(factor float64) Money {
	return Money{value: m.value.Mul(decimal.NewFromFloat(factor))}
}

// Div 除法.
func (m Money) Div(factor float64) Money {
	return Money{value: m.value.Div(decimal.NewFromFloat(factor))}
}

// Format 格式化为指定位数的字符串.
func (m Money) Format(places int32) string {
	return m.value.StringFixed(places)
}

// FenToYuan 兼容旧接口：将分为单位的整数转换为元.
func FenToYuan(fen int64) float64 {
	return NewFromFen(fen).ToFloat()
}

// YuanToFen 兼容旧接口：将元转换为分为单位的整数.
func YuanToFen(yuan float64) int64 {
	return New(yuan).ToFen()
}

// FormatMoney 兼容旧接口：格式化分为单位的金额.
func FormatMoney(fen int64) string {
	return NewFromFen(fen).String()
}
