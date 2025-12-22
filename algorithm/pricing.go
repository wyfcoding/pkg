// Package algos - 期权定价算法（Black-Scholes 模型）
package algorithm

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// BlackScholesCalculator Black-Scholes 期权定价计算器
type BlackScholesCalculator struct{}

// NewBlackScholesCalculator 创建 Black-Scholes 计算器
func NewBlackScholesCalculator() *BlackScholesCalculator {
	return &BlackScholesCalculator{}
}

// CalculateCallPrice 计算看涨期权价格
// 参数：
//   - S: 标的资产当前价格
//   - K: 行权价格
//   - T: 到期时间（年）
//   - r: 无风险利率
//   - sigma: 波动率
//   - q: 股息收益率（可选，默认为 0）
//
// 返回：期权价格
func (bsc *BlackScholesCalculator) CalculateCallPrice(S, K, T, r, sigma, q decimal.Decimal) (decimal.Decimal, error) {
	// 验证输入
	if S.LessThanOrEqual(decimal.Zero) || K.LessThanOrEqual(decimal.Zero) || T.LessThanOrEqual(decimal.Zero) || sigma.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("invalid input parameters")
	}

	// 转换为 float64 进行计算
	sFloat := S.InexactFloat64()
	kFloat := K.InexactFloat64()
	tFloat := T.InexactFloat64()
	rFloat := r.InexactFloat64()
	sigmaFloat := sigma.InexactFloat64()
	qFloat := q.InexactFloat64()

	// 计算 d1 和 d2
	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	d2 := d1 - sigmaFloat*math.Sqrt(tFloat)

	// 计算看涨期权价格
	callPrice := sFloat*math.Exp(-qFloat*tFloat)*normCDF(d1) - kFloat*math.Exp(-rFloat*tFloat)*normCDF(d2)

	return decimal.NewFromFloat(callPrice), nil
}

// CalculatePutPrice 计算看跌期权价格
func (bsc *BlackScholesCalculator) CalculatePutPrice(S, K, T, r, sigma, q decimal.Decimal) (decimal.Decimal, error) {
	// 验证输入
	if S.LessThanOrEqual(decimal.Zero) || K.LessThanOrEqual(decimal.Zero) || T.LessThanOrEqual(decimal.Zero) || sigma.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, fmt.Errorf("invalid input parameters")
	}

	// 转换为 float64 进行计算
	sFloat := S.InexactFloat64()
	kFloat := K.InexactFloat64()
	tFloat := T.InexactFloat64()
	rFloat := r.InexactFloat64()
	sigmaFloat := sigma.InexactFloat64()
	qFloat := q.InexactFloat64()

	// 计算 d1 和 d2
	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	d2 := d1 - sigmaFloat*math.Sqrt(tFloat)

	// 计算看跌期权价格
	putPrice := kFloat*math.Exp(-rFloat*tFloat)*normCDF(-d2) - sFloat*math.Exp(-qFloat*tFloat)*normCDF(-d1)

	return decimal.NewFromFloat(putPrice), nil
}

// CalculateDelta 计算 Delta（期权价格对标的资产价格的敏感度）
func (bsc *BlackScholesCalculator) CalculateDelta(optionType string, S, K, T, r, sigma, q decimal.Decimal) (decimal.Decimal, error) {
	sFloat := S.InexactFloat64()
	kFloat := K.InexactFloat64()
	tFloat := T.InexactFloat64()
	rFloat := r.InexactFloat64()
	sigmaFloat := sigma.InexactFloat64()
	qFloat := q.InexactFloat64()

	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))

	var delta float64
	switch optionType {
	case "CALL":
		delta = math.Exp(-qFloat*tFloat) * normCDF(d1)
	case "PUT":
		delta = math.Exp(-qFloat*tFloat) * (normCDF(d1) - 1)
	default:
		return decimal.Zero, fmt.Errorf("invalid option type")
	}

	return decimal.NewFromFloat(delta), nil
}

// CalculateGamma 计算 Gamma（Delta 对标的资产价格的敏感度）
func (bsc *BlackScholesCalculator) CalculateGamma(S, K, T, r, sigma, q decimal.Decimal) (decimal.Decimal, error) {
	sFloat := S.InexactFloat64()
	kFloat := K.InexactFloat64()
	tFloat := T.InexactFloat64()
	rFloat := r.InexactFloat64()
	sigmaFloat := sigma.InexactFloat64()
	qFloat := q.InexactFloat64()

	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))

	gamma := math.Exp(-qFloat*tFloat) * normPDF(d1) / (sFloat * sigmaFloat * math.Sqrt(tFloat))

	return decimal.NewFromFloat(gamma), nil
}

// CalculateVega 计算 Vega（期权价格对波动率的敏感度）
func (bsc *BlackScholesCalculator) CalculateVega(S, K, T, r, sigma, q decimal.Decimal) (decimal.Decimal, error) {
	sFloat := S.InexactFloat64()
	kFloat := K.InexactFloat64()
	tFloat := T.InexactFloat64()
	rFloat := r.InexactFloat64()
	sigmaFloat := sigma.InexactFloat64()
	qFloat := q.InexactFloat64()

	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))

	vega := sFloat * math.Exp(-qFloat*tFloat) * normPDF(d1) * math.Sqrt(tFloat) / 100

	return decimal.NewFromFloat(vega), nil
}

// CalculateTheta 计算 Theta（期权价格对时间的敏感度）
func (bsc *BlackScholesCalculator) CalculateTheta(optionType string, S, K, T, r, sigma, q decimal.Decimal) (decimal.Decimal, error) {
	sFloat := S.InexactFloat64()
	kFloat := K.InexactFloat64()
	tFloat := T.InexactFloat64()
	rFloat := r.InexactFloat64()
	sigmaFloat := sigma.InexactFloat64()
	qFloat := q.InexactFloat64()

	d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	d2 := d1 - sigmaFloat*math.Sqrt(tFloat)

	var theta float64
	switch optionType {
	case "CALL":
		theta = -sFloat*math.Exp(-qFloat*tFloat)*normPDF(d1)*sigmaFloat/(2*math.Sqrt(tFloat)) - rFloat*kFloat*math.Exp(-rFloat*tFloat)*normCDF(d2) + qFloat*sFloat*math.Exp(-qFloat*tFloat)*normCDF(d1)
	case "PUT":
		theta = -sFloat*math.Exp(-qFloat*tFloat)*normPDF(d1)*sigmaFloat/(2*math.Sqrt(tFloat)) + rFloat*kFloat*math.Exp(-rFloat*tFloat)*normCDF(-d2) - qFloat*sFloat*math.Exp(-qFloat*tFloat)*normCDF(-d1)
	default:
		return decimal.Zero, fmt.Errorf("invalid option type")
	}

	return decimal.NewFromFloat(theta / 365), nil // 转换为每日 theta
}

// CalculateRho 计算 Rho（期权价格对利率的敏感度）
func (bsc *BlackScholesCalculator) CalculateRho(optionType string, S, K, T, r, sigma, q decimal.Decimal) (decimal.Decimal, error) {
	sFloat := S.InexactFloat64()
	kFloat := K.InexactFloat64()
	tFloat := T.InexactFloat64()
	rFloat := r.InexactFloat64()
	sigmaFloat := sigma.InexactFloat64()

	d1 := (math.Log(sFloat/kFloat) + (rFloat-q.InexactFloat64()+0.5*sigmaFloat*sigmaFloat)*tFloat) / (sigmaFloat * math.Sqrt(tFloat))
	d2 := d1 - sigmaFloat*math.Sqrt(tFloat)

	var rho float64
	switch optionType {
	case "CALL":
		rho = kFloat * tFloat * math.Exp(-rFloat*tFloat) * normCDF(d2) / 100
	case "PUT":
		rho = -kFloat * tFloat * math.Exp(-rFloat*tFloat) * normCDF(-d2) / 100
	default:
		return decimal.Zero, fmt.Errorf("invalid option type")
	}

	return decimal.NewFromFloat(rho), nil
}

// normCDF 标准正态分布累积分布函数
func normCDF(x float64) float64 {
	return (1.0 + math.Erf(x/math.Sqrt2)) / 2.0
}

// normPDF 标准正态分布概率密度函数
func normPDF(x float64) float64 {
	return math.Exp(-x*x/2) / math.Sqrt(2*math.Pi)
}

// CalculateImpliedVolatility 计算隐含波动率（使用牛顿法）
func (bsc *BlackScholesCalculator) CalculateImpliedVolatility(optionType string, S, K, T, r, q, marketPrice decimal.Decimal) (decimal.Decimal, error) {
	sFloat := S.InexactFloat64()
	kFloat := K.InexactFloat64()
	tFloat := T.InexactFloat64()
	rFloat := r.InexactFloat64()
	qFloat := q.InexactFloat64()
	marketPriceFloat := marketPrice.InexactFloat64()

	// 初始猜测
	sigma := 0.3
	tolerance := 0.0001
	maxIterations := 100

	for range maxIterations {
		// 计算当前价格和 vega
		d1 := (math.Log(sFloat/kFloat) + (rFloat-qFloat+0.5*sigma*sigma)*tFloat) / (sigma * math.Sqrt(tFloat))
		d2 := d1 - sigma*math.Sqrt(tFloat)

		var price, vega float64
		if optionType == "CALL" {
			price = sFloat*math.Exp(-qFloat*tFloat)*normCDF(d1) - kFloat*math.Exp(-rFloat*tFloat)*normCDF(d2)
		} else {
			price = kFloat*math.Exp(-rFloat*tFloat)*normCDF(-d2) - sFloat*math.Exp(-qFloat*tFloat)*normCDF(-d1)
		}

		vega = sFloat * math.Exp(-qFloat*tFloat) * normPDF(d1) * math.Sqrt(tFloat)

		// 牛顿法更新
		diff := price - marketPriceFloat
		if math.Abs(diff) < tolerance {
			return decimal.NewFromFloat(sigma), nil
		}

		if vega == 0 {
			break
		}

		sigma = sigma - diff/vega
		if sigma < 0 {
			sigma = 0.001
		}
	}

	return decimal.NewFromFloat(sigma), nil
}
