package finance

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math"

	algomath "github.com/wyfcoding/pkg/algorithm/math"
)

// LSMPricer 实现了 Longstaff-Schwartz (LSM) 算法
type LSMPricer struct {
	Degree int // 回归多项式的阶数
}

func NewLSMPricer(degree int) *LSMPricer {
	if degree <= 0 {
		degree = 2
	}
	return &LSMPricer{Degree: degree}
}

// AmericanOptionParams 核心定价参数
type AmericanOptionParams struct {
	S0    float64
	K     float64
	T     float64
	R     float64
	Sigma float64
	IsPut bool
	Paths int
	Steps int
}

// ComputePrice 计算美国期权现值
func (p *LSMPricer) ComputePrice(params AmericanOptionParams) (float64, error) {
	dt := params.T / float64(params.Steps)
	df := math.Exp(-params.R * dt)

	// 1. 生成路径
	// G404 Fix: Use crypto/rand for simulation (slow but safe)
	paths := make([][]float64, params.Paths)
	for i := range paths {
		paths[i] = make([]float64, params.Steps+1)
		paths[i][0] = params.S0
		for j := 1; j <= params.Steps; j++ {
			// Box-Muller transform using crypto/rand
			z := p.cryptoNormFloat64()
			paths[i][j] = paths[i][j-1] * math.Exp((params.R-0.5*math.Pow(params.Sigma, 2))*dt+params.Sigma*math.Sqrt(dt)*z)
		}
	}

	// 2. 初始化末端收益
	cashFlows := make([]float64, params.Paths)
	for i := range cashFlows {
		cashFlows[i] = p.payoff(paths[i][params.Steps], params.K, params.IsPut)
	}

	// 3. 反向回归
	for t := params.Steps - 1; t > 0; t-- {
		var xData []float64
		var yData []float64
		var indices []int

		for i := range params.Paths {
			s := paths[i][t]
			iv := p.payoff(s, params.K, params.IsPut)
			if iv > 0 { // 仅考虑价内路径
				xData = append(xData, s)
				yData = append(yData, cashFlows[i]*math.Exp(-params.R*dt))
				indices = append(indices, i)
			} else {
				cashFlows[i] *= df
			}
		}

		if len(indices) > p.Degree+1 {
			coeffs, err := p.regress(xData, yData)
			if err != nil {
				return 0, fmt.Errorf("regression error: %w", err)
			}

			// 比较行权价值与预测的等待价值
			for idx, i := range indices {
				s := xData[idx]
				iv := p.payoff(s, params.K, params.IsPut)

				// 预测延续价值 (Continuation Value)
				cv := 0.0
				for d := 0; d <= p.Degree; d++ {
					cv += coeffs[d] * math.Pow(s, float64(d))
				}

				if iv >= cv {
					cashFlows[i] = iv
				} else {
					cashFlows[i] *= df
				}
			}
		}
	}

	total := 0.0
	for _, cf := range cashFlows {
		total += cf
	}

	return (total / float64(params.Paths)) * df, nil
}

func (p *LSMPricer) payoff(s, k float64, isPut bool) float64 {
	if isPut {
		return math.Max(0, k-s)
	}
	return math.Max(0, s-k)
}

func (p *LSMPricer) regress(x, y []float64) ([]float64, error) {
	n := len(x)
	m := p.Degree + 1

	// A 是 Vandermonde 矩阵 [n x m]
	A := algomath.NewMatrix(n, m)
	for i := range x {
		for j := range y {
			A.Set(i, j, math.Pow(x[i], float64(j)))
		}
	}

	AT := A.Transpose()
	// ATA = A^T * A [m x m]
	ATA, _ := AT.Multiply(A)
	// ATy = A^T * y [m x 1]
	ATy, _ := AT.MultiplyVector(y)

	// 求解线性方程组 ATA * coeffs = ATy
	return ATA.SolveCholesky(ATy)
}

func (p *LSMPricer) cryptoNormFloat64() float64 {
	// Box-Muller transform
	// Generate u1, u2 in (0, 1]
	u1 := p.cryptoFloat64()
	u2 := p.cryptoFloat64()

	R := math.Sqrt(-2.0 * math.Log(u1))
	Theta := 2.0 * math.Pi * u2
	return R * math.Cos(Theta)
}

func (p *LSMPricer) cryptoFloat64() float64 {
	// Generate random float in (0, 1]
	// 53 bits of precision
	b := make([]byte, 8)
	_, _ = crand.Read(b)
	i := binary.LittleEndian.Uint64(b)
	return float64(i&(1<<53-1)+1) / (1 << 53)
}
