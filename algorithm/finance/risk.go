// Package algos - 风险管理算.
package finance

import (
	"math"
	"slices"

	"github.com/shopspring/decimal"
	"github.com/wyfcoding/pkg/xerrors"
)

// RiskCalculator 风险计算.
type RiskCalculator struct{}

// NewRiskCalculator 创建风险计算.
func NewRiskCalculator() *RiskCalculator {
	return &RiskCalculator{}
}

// CalculateVaR 计算风险价值（Value at Risk.
// 参数.
//   - returns: 历史收益率列.
//   - confidenceLevel: 置信水平（如 0.95 表示 95%.
//
// 返回：VaR .
func (rc *RiskCalculator) CalculateVaR(returns []decimal.Decimal, confidenceLevel float64) (decimal.Decimal, error) {
	if len(returns) == 0 {
		return decimal.Zero, xerrors.ErrEmptyData
	}

	// 转换为 float6.
	floatReturns := make([]float64, len(returns))
	for i, r := range returns {
		floatReturns[i] = r.InexactFloat64()
	}

	// 排.
	slices.Sort(floatReturns)

	// 计算 VaR（历史方法.
	index := int(float64(len(floatReturns)) * (1 - confidenceLevel))
	if index >= len(floatReturns) {
		index = len(floatReturns) - 1
	}

	return decimal.NewFromFloat(floatReturns[index]), nil
}

// CalculateCVaR 计算条件风险价值（Conditional Value at Risk.
// 也称为 Expected Shortfal.
func (rc *RiskCalculator) CalculateCVaR(returns []decimal.Decimal, confidenceLevel float64) (decimal.Decimal, error) {
	if len(returns) == 0 {
		return decimal.Zero, xerrors.ErrEmptyData
	}

	// 转换为 float6.
	floatReturns := make([]float64, len(returns))
	for i, r := range returns {
		floatReturns[i] = r.InexactFloat64()
	}

	// 排.
	slices.Sort(floatReturns)

	// 计算 CVa.
	index := int(float64(len(floatReturns)) * (1 - confidenceLevel))
	if index >= len(floatReturns) {
		index = len(floatReturns) - 1
	}

	sum := 0.0
	for i := 0; i <= index; i++ {
		sum += floatReturns[i]
	}

	cvar := sum / float64(index+1)
	return decimal.NewFromFloat(cvar), nil
}

// CalculateMaxDrawdown 计算最大回.
func (rc *RiskCalculator) CalculateMaxDrawdown(prices []decimal.Decimal) (decimal.Decimal, error) {
	if len(prices) == 0 {
		return decimal.Zero, xerrors.ErrEmptyData
	}

	maxPrice := prices[0]
	maxDrawdown := decimal.Zero

	for _, price := range prices {
		if price.GreaterThan(maxPrice) {
			maxPrice = price
		}

		drawdown := maxPrice.Sub(price).Div(maxPrice)
		if drawdown.GreaterThan(maxDrawdown) {
			maxDrawdown = drawdown
		}
	}

	return maxDrawdown, nil
}

// CalculateSharpeRatio 计算夏普比.
// 参数.
//   - returns: 收益率列.
//   - riskFreeRate: 无风险利.
//
// 返回：夏普比.
func (rc *RiskCalculator) CalculateSharpeRatio(returns []decimal.Decimal, riskFreeRate decimal.Decimal) (decimal.Decimal, error) {
	if len(returns) == 0 {
		return decimal.Zero, xerrors.ErrEmptyData
	}

	// 计算平均收.
	sum := decimal.Zero
	for _, r := range returns {
		sum = sum.Add(r)
	}
	avgReturn := sum.Div(decimal.NewFromInt(int64(len(returns))))

	// 计算标准.
	varianceSum := decimal.Zero
	for _, r := range returns {
		diff := r.Sub(avgReturn)
		varianceSum = varianceSum.Add(diff.Mul(diff))
	}
	variance := varianceSum.Div(decimal.NewFromInt(int64(len(returns))))
	stdDev := decimal.NewFromFloat(math.Sqrt(variance.InexactFloat64()))

	if stdDev.IsZero() {
		return decimal.Zero, xerrors.ErrZeroVariance
	}

	// 计算夏普比.
	sharpeRatio := avgReturn.Sub(riskFreeRate).Div(stdDev)
	return sharpeRatio, nil
}

// CalculateVolatility 计算波动.
func (rc *RiskCalculator) CalculateVolatility(returns []decimal.Decimal) (decimal.Decimal, error) {
	if len(returns) == 0 {
		return decimal.Zero, xerrors.ErrEmptyData
	}

	// 计算平均收.
	sum := decimal.Zero
	for _, r := range returns {
		sum = sum.Add(r)
	}
	avgReturn := sum.Div(decimal.NewFromInt(int64(len(returns))))

	// 计算方.
	varianceSum := decimal.Zero
	for _, r := range returns {
		diff := r.Sub(avgReturn)
		varianceSum = varianceSum.Add(diff.Mul(diff))
	}
	variance := varianceSum.Div(decimal.NewFromInt(int64(len(returns))))

	// 计算标准差（年化.
	stdDev := decimal.NewFromFloat(math.Sqrt(variance.InexactFloat64()))
	annualizedVolatility := stdDev.Mul(decimal.NewFromFloat(math.Sqrt(252))) // 252 个交易.

	return annualizedVolatility, nil
}

// CalculateCorrelation 计算两个资产的相关系.
func (rc *RiskCalculator) CalculateCorrelation(returns1, returns2 []decimal.Decimal) (decimal.Decimal, error) {
	if len(returns1) != len(returns2) || len(returns1) == 0 {
		return decimal.Zero, xerrors.ErrInvalidInput
	}

	// 计算平均.
	sum1 := decimal.Zero
	sum2 := decimal.Zero
	for i := range returns1 {
		sum1 = sum1.Add(returns1[i])
		sum2 = sum2.Add(returns2[i])
	}
	avg1 := sum1.Div(decimal.NewFromInt(int64(len(returns1))))
	avg2 := sum2.Div(decimal.NewFromInt(int64(len(returns2))))

	// 计算协方差和标准.
	covSum := decimal.Zero
	var1Sum := decimal.Zero
	var2Sum := decimal.Zero

	for i := range returns1 {
		diff1 := returns1[i].Sub(avg1)
		diff2 := returns2[i].Sub(avg2)
		covSum = covSum.Add(diff1.Mul(diff2))
		var1Sum = var1Sum.Add(diff1.Mul(diff1))
		var2Sum = var2Sum.Add(diff2.Mul(diff2))
	}

	cov := covSum.Div(decimal.NewFromInt(int64(len(returns1))))
	std1 := decimal.NewFromFloat(math.Sqrt(var1Sum.Div(decimal.NewFromInt(int64(len(returns1)))).InexactFloat64()))
	std2 := decimal.NewFromFloat(math.Sqrt(var2Sum.Div(decimal.NewFromInt(int64(len(returns2)))).InexactFloat64()))

	if std1.IsZero() || std2.IsZero() {
		return decimal.Zero, xerrors.ErrZeroVariance
	}

	correlation := cov.Div(std1.Mul(std2))
	return correlation, nil
}

// CalculateBeta 计算 Beta（相对于市场的系统风险.
func (rc *RiskCalculator) CalculateBeta(assetReturns, marketReturns []decimal.Decimal) (decimal.Decimal, error) {
	if len(assetReturns) != len(marketReturns) || len(assetReturns) == 0 {
		return decimal.Zero, xerrors.ErrInvalidInput
	}

	// 计算协方.
	covariance, err := rc.calculateCovariance(assetReturns, marketReturns)
	if err != nil {
		return decimal.Zero, err
	}

	// 计算市场方.
	marketVariance, err := rc.calculateVariance(marketReturns)
	if err != nil {
		return decimal.Zero, err
	}

	if marketVariance.IsZero() {
		return decimal.Zero, xerrors.ErrZeroVariance
	}

	beta := covariance.Div(marketVariance)
	return beta, nil
}

// calculateCovariance 计算协方.
func (rc *RiskCalculator) calculateCovariance(x, y []decimal.Decimal) (decimal.Decimal, error) {
	if len(x) != len(y) || len(x) == 0 {
		return decimal.Zero, xerrors.ErrInvalidInput
	}

	// 计算平均.
	sumX := decimal.Zero
	sumY := decimal.Zero
	for i := range x {
		sumX = sumX.Add(x[i])
		sumY = sumY.Add(y[i])
	}
	avgX := sumX.Div(decimal.NewFromInt(int64(len(x))))
	avgY := sumY.Div(decimal.NewFromInt(int64(len(y))))

	// 计算协方.
	covSum := decimal.Zero
	for i := range x {
		covSum = covSum.Add(x[i].Sub(avgX).Mul(y[i].Sub(avgY)))
	}

	return covSum.Div(decimal.NewFromInt(int64(len(x)))), nil
}

// calculateVariance 计算方.
func (rc *RiskCalculator) calculateVariance(data []decimal.Decimal) (decimal.Decimal, error) {
	if len(data) == 0 {
		return decimal.Zero, xerrors.ErrEmptyData
	}

	// 计算平均.
	sum := decimal.Zero
	for _, d := range data {
		sum = sum.Add(d)
	}
	avg := sum.Div(decimal.NewFromInt(int64(len(data))))

	// 计算方.
	varSum := decimal.Zero
	for _, d := range data {
		diff := d.Sub(avg)
		varSum = varSum.Add(diff.Mul(diff))
	}

	return varSum.Div(decimal.NewFromInt(int64(len(data)))), nil
}

// EvaluateFraudScore 根据多准则决策模型计算反欺诈风险评分。
// factors: 各维度的风险原始分 (0-1)。
// weights: 各维度的重要性权重 (之和应为 1)。
// 返回 0-1 之间的最终风险评分。
func (rc *RiskCalculator) EvaluateFraudScore(factors, weights map[string]float64) float64 {
	if len(factors) == 0 {
		return 0.0
	}

	totalScore := 0.0
	weightSum := 0.0

	for key, score := range factors {
		w, ok := weights[key]
		if !ok {
			w = 1.0 / float64(len(factors)) // 默认等权.
		}
		totalScore += score * w
		weightSum += w
	}

	if weightSum > 0 {
		totalScore /= weightSum
	}

	// 归一化到 0-1 范.
	if totalScore > 1.0 {
		totalScore = 1.0
	} else if totalScore < 0 {
		totalScore = 0
	}

	return totalScore
}
