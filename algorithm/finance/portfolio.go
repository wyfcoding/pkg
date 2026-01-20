package finance

import (
	"math"

	"github.com/shopspring/decimal"
	algomath "github.com/wyfcoding/pkg/algorithm/math"
)

// PortfolioOptimizer 组合优化器
type PortfolioOptimizer struct {
	Assets     []string
	Returns    []decimal.Decimal   // 预期收益率
	Covariance [][]decimal.Decimal // 协方差矩阵
}

func NewPortfolioOptimizer(assets []string, returns []decimal.Decimal, cov [][]decimal.Decimal) *PortfolioOptimizer {
	return &PortfolioOptimizer{
		Assets:     assets,
		Returns:    returns,
		Covariance: cov,
	}
}

// OptimizeMeanVariance 均值-方差优化 (简单闭式解示例: 最小方差组合)
// 实际中通常需要二次编程 (Quadratic Programming) 来处理约束.
func (o *PortfolioOptimizer) OptimizeMinimumVariance() map[string]decimal.Decimal {
	n := len(o.Assets)
	if n == 0 {
		return nil
	}

	// 最小化 w'Σw s.t. Σw = 1
	// 解为 w = Σ^-1 * 1 / (1' * Σ^-1 * 1)

	// 1. 将协方差矩阵转换为 float64 矩阵进行线性代数运算
	matrixData := make([][]float64, n)
	for i := range o.Covariance {
		matrixData[i] = make([]float64, n)
		for j := range o.Covariance[i] {
			matrixData[i][j] = o.Covariance[i][j].InexactFloat64()
		}
	}

	sigma, err := algomath.NewMatrixFromData(matrixData)
	if err != nil {
		return o.EqualWeight()
	}

	ones := make([]float64, n)
	for i := range ones {
		ones[i] = 1.0
	}

	// 解 Sigma * w_raw = ones
	wRaw, err := sigma.SolveCholesky(ones)
	if err != nil {
		return o.EqualWeight()
	}

	// sum(w_raw)
	sumWRaw := 0.0
	for _, w := range wRaw {
		sumWRaw += w
	}

	// Normalize
	weights := make(map[string]decimal.Decimal)
	for i, asset := range o.Assets {
		weights[asset] = decimal.NewFromFloat(wRaw[i] / sumWRaw)
	}

	return weights
}

// EqualWeight 等权重分配
func (o *PortfolioOptimizer) EqualWeight() map[string]decimal.Decimal {
	n := len(o.Assets)
	weight := decimal.NewFromFloat(1.0 / float64(n))
	weights := make(map[string]decimal.Decimal)
	for _, asset := range o.Assets {
		weights[asset] = weight
	}
	return weights
}

// CalculatePortfolioRisk 计算组合风险 (标准差)
func (o *PortfolioOptimizer) CalculatePortfolioRisk(weights map[string]decimal.Decimal) decimal.Decimal {
	var variance float64
	for i, a1 := range o.Assets {
		w1 := weights[a1].InexactFloat64()
		for j, a2 := range o.Assets {
			w2 := weights[a2].InexactFloat64()
			cov := o.Covariance[i][j].InexactFloat64()
			variance += w1 * w2 * cov
		}
	}
	return decimal.NewFromFloat(math.Sqrt(variance))
}
