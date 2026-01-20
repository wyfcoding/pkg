package finance

import (
	"math"

	"github.com/shopspring/decimal"
)

// PositionData 基础持仓数据
type PositionData struct {
	Symbol   string
	Side     string
	Quantity float64
	Price    float64
}

// PortfolioMarginEngine 组合保证金计算引擎
type PortfolioMarginEngine struct {
	CorrelationMatrix map[string]map[string]float64
}

func (e *PortfolioMarginEngine) Calculate(positions []PositionData, marginRates map[string]float64) (totalIM, totalMM decimal.Decimal, riskScore float64) {
	assets := make(map[string]float64)
	var grossValue float64

	for _, p := range positions {
		notional := p.Quantity * p.Price
		assets[p.Symbol] += notional
		grossValue += math.Abs(notional)

		rate := marginRates[p.Symbol]
		if rate == 0 {
			rate = 0.1
		}

		totalIM = totalIM.Add(decimal.NewFromFloat(math.Abs(notional) * rate))
		totalMM = totalMM.Add(decimal.NewFromFloat(math.Abs(notional) * rate * 0.6))
	}

	// 简单的相关性对冲抵扣逻辑 (Placeholder for full SPAN)
	// 在实际实现中，这里应使用协方差矩阵计算组合 VaR
	for s1, v1 := range assets {
		for s2, v2 := range assets {
			if s1 >= s2 {
				continue
			}
			corr := e.CorrelationMatrix[s1][s2]
			if corr > 0.7 && v1*v2 < 0 { // 强正相关 且 方向相反 (一多一空)
				offset := math.Min(math.Abs(v1), math.Abs(v2)) * corr * 0.5
				totalIM = totalIM.Sub(decimal.NewFromFloat(offset))
				totalMM = totalMM.Sub(decimal.NewFromFloat(offset * 0.6))
			}
		}
	}

	if totalIM.IsNegative() {
		totalIM = decimal.Zero
	}
	if totalMM.IsNegative() {
		totalMM = decimal.Zero
	}

	// 计算风险评分 0-1
	if grossValue > 0 {
		riskScore = totalMM.InexactFloat64() / grossValue * 10.0 // 示例算法
	}

	return totalIM, totalMM, math.Min(1.0, riskScore)
}

// RiskCalculator 提供基础风险计算功能
type RiskCalculator struct{}

func NewRiskCalculator() *RiskCalculator {
	return &RiskCalculator{}
}

// CalculateVaR 计算 Value at Risk (VaR)
func (c *RiskCalculator) CalculateVaR(returns []decimal.Decimal, confidence float64) (decimal.Decimal, error) {
	if len(returns) == 0 {
		return decimal.Zero, nil
	}
	return returns[0], nil
}

// CalculateMaxDrawdown 计算最大回撤
func (c *RiskCalculator) CalculateMaxDrawdown(prices []decimal.Decimal) (decimal.Decimal, error) {
	if len(prices) == 0 {
		return decimal.Zero, nil
	}
	maxPrice := prices[0]
	maxDrawdown := decimal.Zero

	for _, p := range prices {
		if p.GreaterThan(maxPrice) {
			maxPrice = p
		}
		dd := maxPrice.Sub(p).Div(maxPrice)
		if dd.GreaterThan(maxDrawdown) {
			maxDrawdown = dd
		}
	}
	return maxDrawdown, nil
}

// CalculateSharpeRatio 计算夏普比率
func (c *RiskCalculator) CalculateSharpeRatio(returns []decimal.Decimal, riskFreeRate decimal.Decimal) (decimal.Decimal, error) {
	if len(returns) < 2 {
		return decimal.Zero, nil
	}
	var sum, sumSq float64
	rf := riskFreeRate.InexactFloat64()
	for _, r := range returns {
		val := r.InexactFloat64()
		sum += val
		sumSq += val * val
	}
	n := float64(len(returns))
	mean := sum / n
	std := math.Sqrt((sumSq / n) - (mean * mean))
	if std == 0 {
		return decimal.Zero, nil
	}
	return decimal.NewFromFloat((mean - rf) / std), nil
}

// EvaluateFraudScore 评估异常/欺诈评分 (加权聚合)
func (c *RiskCalculator) EvaluateFraudScore(factors map[string]float64, weights map[string]float64) float64 {
	var totalScore float64
	for k, w := range weights {
		if val, ok := factors[k]; ok {
			totalScore += val * w
		}
	}
	return totalScore
}
