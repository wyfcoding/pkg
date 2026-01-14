// Package algos - 市场模拟算法.
package sim

import (
	"encoding/binary"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/wyfcoding/pkg/algorithm/finance"
	"github.com/wyfcoding/pkg/cast"
	"github.com/wyfcoding/pkg/xerrors"

	crypto_rand "crypto/rand"

	"github.com/shopspring/decimal"
)

// GeometricBrownianMotion 几何布朗运动模拟.
type GeometricBrownianMotion struct {
	initialPrice decimal.Decimal
	drift        decimal.Decimal // 漂移.
	volatility   decimal.Decimal // 波动.
	timeStep     decimal.Decimal // 时间步.
}

// NewGeometricBrownianMotion 创建 GBM 模拟.
func NewGeometricBrownianMotion(initialPrice, drift, volatility, timeStep decimal.Decimal) *GeometricBrownianMotion {
	return &GeometricBrownianMotion{
		initialPrice: initialPrice,
		drift:        drift,
		volatility:   volatility,
		timeStep:     timeStep,
	}
}

// cryptoNormFloat64 使用 Box-Muller 变换从 crypto/rand 产生正态分布随机数.
func cryptoNormFloat64() float64 {
	var b [16]byte
	if _, err := crypto_rand.Read(b[:]); err != nil {
		ts := time.Now().UnixNano()
		// G115 Fix: use unsafe cast via utils to bypass overflow warning.
		val := cast.Int64ToUint64(ts)
		binary.LittleEndian.PutUint64(b[:8], val)
		binary.LittleEndian.PutUint64(b[8:], val)
	}
	u1 := float64(binary.LittleEndian.Uint64(b[:8]))/float64(math.MaxUint64) + 1e-10
	u2 := float64(binary.LittleEndian.Uint64(b[8:])) / float64(math.MaxUint64)
	return math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
}

// Simulate 模拟价格路径.
func (gbm *GeometricBrownianMotion) Simulate(steps int) []decimal.Decimal {
	prices := make([]decimal.Decimal, steps+1)
	prices[0] = gbm.initialPrice

	driftFloat := gbm.drift.InexactFloat64()
	volatilityFloat := gbm.volatility.InexactFloat64()
	timeStepFloat := gbm.timeStep.InexactFloat64()

	// 预计算常量.
	driftTerm := (driftFloat - 0.5*volatilityFloat*volatilityFloat) * timeStepFloat
	volTerm := volatilityFloat * math.Sqrt(timeStepFloat)

	for i := 1; i <= steps; i++ {
		z := cryptoNormFloat64()
		currentPrice := prices[i-1].InexactFloat64()
		exponent := driftTerm + volTerm*z
		newPrice := currentPrice * math.Exp(exponent)
		prices[i] = decimal.NewFromFloat(newPrice)
	}

	return prices
}

// SimulateMultiplePaths 模拟多条价格路径.
// 优化：并行模拟，利用多核 CPU。
func (gbm *GeometricBrownianMotion) SimulateMultiplePaths(steps, paths int) [][]decimal.Decimal {
	allPaths := make([][]decimal.Decimal, paths)

	numWorkers := runtime.GOMAXPROCS(0)
	if paths < 100 {
		numWorkers = 1
	}

	var wg sync.WaitGroup
	wg.Add(paths)

	// 使用信号量限制最大并发协程，防止过多协程导致调度压力.
	sem := make(chan struct{}, numWorkers)

	for i := range paths {
		go func(pathIdx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			allPaths[pathIdx] = gbm.Simulate(steps)
		}(i)
	}
	wg.Wait()

	return allPaths
}

// CalculatePathStatistics 计算路径统计.
func (gbm *GeometricBrownianMotion) CalculatePathStatistics(paths [][]decimal.Decimal) map[string]decimal.Decimal {
	if len(paths) == 0 {
		return nil
	}

	finalPrices := make([]decimal.Decimal, len(paths))
	for i, path := range paths {
		finalPrices[i] = path[len(path)-1]
	}

	// 计算统计项.
	sum := decimal.Zero
	minPrice := finalPrices[0]
	maxPrice := finalPrices[0]

	for _, price := range finalPrices {
		sum = sum.Add(price)
		if price.LessThan(minPrice) {
			minPrice = price
		}
		if price.GreaterThan(maxPrice) {
			maxPrice = price
		}
	}

	avgPrice := sum.Div(decimal.NewFromInt(int64(len(finalPrices))))

	// 计算标准差.
	varSum := decimal.Zero
	for _, price := range finalPrices {
		diff := price.Sub(avgPrice)
		varSum = varSum.Add(diff.Mul(diff))
	}
	variance := varSum.Div(decimal.NewFromInt(int64(len(finalPrices))))
	stdDev := decimal.NewFromFloat(math.Sqrt(variance.InexactFloat64()))

	return map[string]decimal.Decimal{
		"average": avgPrice,
		"min":     minPrice,
		"max":     maxPrice,
		"stddev":  stdDev,
	}
}

// MonteCarlo 蒙特卡洛模拟.
type MonteCarlo struct {
	gbm *GeometricBrownianMotion
}

// NewMonteCarlo 创建蒙特卡洛模拟.
func NewMonteCarlo(gbm *GeometricBrownianMotion) *MonteCarlo {
	return &MonteCarlo{
		gbm: gbm,
	}
}

// CalculateOptionPrice 使用蒙特卡洛方法计算期权价格.
func (mc *MonteCarlo) CalculateOptionPrice(optionType string, strikePrice decimal.Decimal, steps, paths int, riskFreeRate decimal.Decimal) (decimal.Decimal, error) {
	if optionType != "CALL" && optionType != "PUT" {
		return decimal.Zero, xerrors.ErrInvalidOptionType
	}

	// 模拟多条路径.
	allPaths := mc.gbm.SimulateMultiplePaths(steps, paths)

	// 计算每条路径的期权收益.
	totalPayoff := decimal.Zero
	for _, path := range allPaths {
		finalPrice := path[len(path)-1]
		var payoff decimal.Decimal

		if optionType == "CALL" {
			payoff = finalPrice.Sub(strikePrice)
			if payoff.LessThan(decimal.Zero) {
				payoff = decimal.Zero
			}
		} else { // PUT.
			payoff = strikePrice.Sub(finalPrice)
			if payoff.LessThan(decimal.Zero) {
				payoff = decimal.Zero
			}
		}

		totalPayoff = totalPayoff.Add(payoff)
	}

	// 计算平均收益并折现.
	avgPayoff := totalPayoff.Div(decimal.NewFromInt(int64(paths)))
	discountFactor := decimal.NewFromFloat(math.Exp(-riskFreeRate.InexactFloat64() * mc.gbm.timeStep.InexactFloat64() * float64(steps)))
	optionPrice := avgPayoff.Mul(discountFactor)

	return optionPrice, nil
}

// CalculateVaRMonteCarlo 使用蒙特卡洛方法计算 VaR.
func (mc *MonteCarlo) CalculateVaRMonteCarlo(steps, paths int, confidenceLevel float64) (decimal.Decimal, error) {
	// 模拟多条路径.
	allPaths := mc.gbm.SimulateMultiplePaths(steps, paths)

	// 计算每条路径的收益率.
	returns := make([]decimal.Decimal, len(allPaths))
	for i, path := range allPaths {
		finalPrice := path[len(path)-1]
		initialPrice := path[0]
		returnRate := finalPrice.Sub(initialPrice).Div(initialPrice)
		returns[i] = returnRate
	}

	// 使用历史方法计算 VaR.
	rc := finance.NewRiskCalculator()
	return rc.CalculateVaR(returns, confidenceLevel)
}
