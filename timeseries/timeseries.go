// 变更说明：
// 新增通用时序数据处理工具，专注于金融 K 线（OHLCV）聚合和技术指标计算。
// 核心功能：
// 1. Tick 级数据流式聚合成不同周期 K 线（1m, 5m, 1h, 1d 等）
// 2. 技术指标计算器（SMA, EMA, MACD, RSI, Bollinger Bands 等）
package timeseries

import (
	"math"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// Tick 实时行情数据点。
type Tick struct {
	Symbol    string
	Price     decimal.Decimal
	Volume    decimal.Decimal
	Timestamp int64 // 纳秒
}

// Bar K 线（OHLCV）数据。
type Bar struct {
	Timestamp int64           `json:"timestamp"` // 周期开放时间戳（秒）
	Symbol    string          `json:"symbol"`
	Open      decimal.Decimal `json:"open"`
	High      decimal.Decimal `json:"high"`
	Low       decimal.Decimal `json:"low"`
	Close     decimal.Decimal `json:"close"`
	Volume    decimal.Decimal `json:"volume"`
	Count     int64           `json:"count"` // 成交笔数
	IsClosed  bool            `json:"is_closed"`
}

// Timeframe K线周期类型。
type Timeframe time.Duration

const (
	M1  Timeframe = Timeframe(time.Minute)
	M5  Timeframe = Timeframe(5 * time.Minute)
	M15 Timeframe = Timeframe(15 * time.Minute)
	H1  Timeframe = Timeframe(time.Hour)
	H4  Timeframe = Timeframe(4 * time.Hour)
	D1  Timeframe = Timeframe(24 * time.Hour)
)

// BarAggregator 流式 K 线聚合器。
type BarAggregator struct {
	timeframe Timeframe
	current   *Bar
}

// NewBarAggregator 创建新的聚合器。
func NewBarAggregator(tf Timeframe) *BarAggregator {
	return &BarAggregator{timeframe: tf}
}

// AddTick 将 Tick 添加入聚合器，如果产生了新的完整 K 线则返回（前一个 K 线）。
// 此方法假设 Tick 的时间戳是单调递增的。
func (a *BarAggregator) AddTick(tick Tick) *Bar {
	tickTime := time.Unix(0, tick.Timestamp)
	barTime := tickTime.Truncate(time.Duration(a.timeframe)).Unix()

	if a.current == nil {
		a.current = &Bar{
			Timestamp: barTime,
			Symbol:    tick.Symbol,
			Open:      tick.Price,
			High:      tick.Price,
			Low:       tick.Price,
			Close:     tick.Price,
			Volume:    tick.Volume,
			Count:     1,
		}
		return nil
	}

	if barTime == a.current.Timestamp {
		// 当前周期内
		if tick.Price.GreaterThan(a.current.High) {
			a.current.High = tick.Price
		}
		if tick.Price.LessThan(a.current.Low) {
			a.current.Low = tick.Price
		}
		a.current.Close = tick.Price
		a.current.Volume = a.current.Volume.Add(tick.Volume)
		a.current.Count++
		return nil
	}

	// 跨周期了，保存并返回上一个 K 线，初始化新 K 线
	closedBar := a.current
	closedBar.IsClosed = true

	a.current = &Bar{
		Timestamp: barTime,
		Symbol:    tick.Symbol,
		Open:      tick.Price,
		High:      tick.Price,
		Low:       tick.Price,
		Close:     tick.Price,
		Volume:    tick.Volume,
		Count:     1,
	}

	return closedBar
}

// GetCurrentBar 获取当前尚未闭合的 K 线拷贝。
func (a *BarAggregator) GetCurrentBar() *Bar {
	if a.current == nil {
		return nil
	}
	b := *a.current
	return &b
}

// ----------------- 技术指标计算 -----------------

// CalculateSMA 计算简单移动平均线 (Simple Moving Average)。
func CalculateSMA(prices []decimal.Decimal, period int) []decimal.Decimal {
	if period <= 0 || len(prices) < period {
		return nil
	}

	result := make([]decimal.Decimal, len(prices)-period+1)
	sum := decimal.Zero

	// 初始化第一个窗口
	for i := 0; i < period; i++ {
		sum = sum.Add(prices[i])
	}
	divisor := decimal.NewFromInt(int64(period))
	result[0] = sum.Div(divisor)

	// 滑动窗口
	for i := period; i < len(prices); i++ {
		sum = sum.Add(prices[i]).Sub(prices[i-period])
		result[i-period+1] = sum.Div(divisor)
	}

	return result
}

// CalculateEMA 计算指数移动平均线 (Exponential Moving Average)。
func CalculateEMA(prices []decimal.Decimal, period int) []decimal.Decimal {
	if period <= 0 || len(prices) < period {
		return nil
	}

	result := make([]decimal.Decimal, len(prices)-period+1)
	multiplier := decimal.NewFromFloat(2.0 / float64(period+1))
	oneMinusMult := decimal.NewFromFloat(1.0).Sub(multiplier)

	// 第一个 EMA 使用 SMA 初始值
	sum := decimal.Zero
	for i := 0; i < period; i++ {
		sum = sum.Add(prices[i])
	}
	ema := sum.Div(decimal.NewFromInt(int64(period)))
	result[0] = ema

	for i := period; i < len(prices); i++ {
		ema = prices[i].Mul(multiplier).Add(ema.Mul(oneMinusMult))
		result[i-period+1] = ema
	}

	return result
}

// MACDResult MACD 结果。
type MACDResult struct {
	MACD      []decimal.Decimal // MACD 线 (Fast EMA - Slow EMA)
	Signal    []decimal.Decimal // Signal 线 (MACD 的 EMA)
	Histogram []decimal.Decimal // 柱状图 (MACD - Signal)
}

// CalculateMACD 计算平滑异同移动平均线 (Moving Average Convergence Divergence)。
// 标准参数: fastPeriod=12, slowPeriod=26, signalPeriod=9
func CalculateMACD(prices []decimal.Decimal, fastPeriod, slowPeriod, signalPeriod int) *MACDResult {
	if len(prices) < slowPeriod+signalPeriod {
		return nil
	}

	fastEMA := CalculateEMA(prices, fastPeriod)
	slowEMA := CalculateEMA(prices, slowPeriod)

	// 对齐长度（丢弃 fastEMA 前面多出的部分）
	diff := len(fastEMA) - len(slowEMA)
	macdLine := make([]decimal.Decimal, len(slowEMA))
	for i := 0; i < len(slowEMA); i++ {
		macdLine[i] = fastEMA[i+diff].Sub(slowEMA[i])
	}

	signalLine := CalculateEMA(macdLine, signalPeriod)

	// 计算 Histogram
	histDiff := len(macdLine) - len(signalLine)
	histogram := make([]decimal.Decimal, len(signalLine))
	for i := 0; i < len(signalLine); i++ {
		histogram[i] = macdLine[i+histDiff].Sub(signalLine[i])
	}

	return &MACDResult{
		MACD:      macdLine[histDiff:], // 截断使得三条线等长
		Signal:    signalLine,
		Histogram: histogram,
	}
}

// CalculateRSI 计算相对强弱指标 (Relative Strength Index)。
// 标准参数: period=14
func CalculateRSI(prices []decimal.Decimal, period int) []decimal.Decimal {
	if period <= 0 || len(prices) <= period {
		return nil
	}

	result := make([]decimal.Decimal, len(prices)-period)

	var avgGain, avgLoss decimal.Decimal
	zero := decimal.Zero
	periodDec := decimal.NewFromInt(int64(period))

	// 第一步：计算初始的平均收益和亏损
	for i := 1; i <= period; i++ {
		change := prices[i].Sub(prices[i-1])
		if change.IsPositive() {
			avgGain = avgGain.Add(change)
		} else {
			avgLoss = avgLoss.Add(change.Abs())
		}
	}
	avgGain = avgGain.Div(periodDec)
	avgLoss = avgLoss.Div(periodDec)

	hundred := decimal.NewFromInt(100)

	if avgLoss.IsZero() {
		result[0] = hundred
	} else {
		rs := avgGain.Div(avgLoss)
		result[0] = hundred.Sub(hundred.Div(decimal.NewFromInt(1).Add(rs)))
	}

	// 增量计算剩余 RSI (平滑)
	for i := period + 1; i < len(prices); i++ {
		change := prices[i].Sub(prices[i-1])
		gain, loss := zero, zero
		if change.IsPositive() {
			gain = change
		} else {
			loss = change.Abs()
		}

		avgGain = avgGain.Mul(periodDec.Sub(decimal.NewFromInt(1))).Add(gain).Div(periodDec)
		avgLoss = avgLoss.Mul(periodDec.Sub(decimal.NewFromInt(1))).Add(loss).Div(periodDec)

		if avgLoss.IsZero() {
			result[i-period] = hundred
		} else {
			rs := avgGain.Div(avgLoss)
			result[i-period] = hundred.Sub(hundred.Div(decimal.NewFromInt(1).Add(rs)))
		}
	}

	return result
}

// BollingerBandsResult 布林带结果。
type BollingerBandsResult struct {
	Upper  []decimal.Decimal
	Middle []decimal.Decimal // SMA
	Lower  []decimal.Decimal
}

// CalculateBollingerBands 计算布林带。
// 标准参数: period=20, multiplier=2.0
func CalculateBollingerBands(prices []decimal.Decimal, period int, multiplier float64) *BollingerBandsResult {
	if period <= 0 || len(prices) < period {
		return nil
	}

	sma := CalculateSMA(prices, period)
	upper := make([]decimal.Decimal, len(sma))
	lower := make([]decimal.Decimal, len(sma))
	multDec := decimal.NewFromFloat(multiplier)

	for i := 0; i < len(sma); i++ {
		// 计算标准差
		var sumSq decimal.Decimal
		mean := sma[i]
		for j := 0; j < period; j++ {
			diff := prices[i+j].Sub(mean)
			sumSq = sumSq.Add(diff.Mul(diff))
		}
		variance, _ := sumSq.Div(decimal.NewFromInt(int64(period))).Float64()
		stdDev := decimal.NewFromFloat(math.Sqrt(variance))

		band := stdDev.Mul(multDec)
		upper[i] = mean.Add(band)
		lower[i] = mean.Sub(band)
	}

	return &BollingerBandsResult{
		Upper:  upper,
		Middle: sma,
		Lower:  lower,
	}
}

// Resample 重新采样 K 线为更大周期 (例如将 1m 采样为 5m)。
func Resample(bars []Bar, targetTimeframe Timeframe) []Bar {
	if len(bars) == 0 {
		return nil
	}

	// 确保按时间排序
	sort.Slice(bars, func(i, j int) bool {
		return bars[i].Timestamp < bars[j].Timestamp
	})

	var result []Bar
	var current *Bar

	for _, b := range bars {
		barTime := time.Unix(b.Timestamp, 0).Truncate(time.Duration(targetTimeframe)).Unix()

		if current == nil {
			newBar := b
			newBar.Timestamp = barTime
			newBar.IsClosed = false
			current = &newBar
			continue
		}

		if barTime == current.Timestamp {
			if b.High.GreaterThan(current.High) {
				current.High = b.High
			}
			if b.Low.LessThan(current.Low) {
				current.Low = b.Low
			}
			current.Close = b.Close
			current.Volume = current.Volume.Add(b.Volume)
			current.Count += b.Count
		} else {
			current.IsClosed = true
			result = append(result, *current)
			newBar := b
			newBar.Timestamp = barTime
			newBar.IsClosed = false
			current = &newBar
		}
	}

	if current != nil {
		current.IsClosed = true
		result = append(result, *current)
	}

	return result
}
