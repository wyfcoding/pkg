package algorithm

import (
	"math"
	"time"
)

// PricingEngine 结构体实现了动态定价引擎。
// 它根据预设的规则和实时因素调整商品价格，旨在优化销售或利润。
type PricingEngine struct {
	basePrice  int64   // 商品的基础价格（单位：分）。
	minPrice   int64   // 商品的最低销售价格，防止价格过低。
	maxPrice   int64   // 商品的最高销售价格，防止价格过高。
	elasticity float64 // 需求价格弹性系数，衡量价格变化对需求量的影响。
}

// NewPricingEngine 创建并返回一个新的 PricingEngine 实例。
func NewPricingEngine(basePrice, minPrice, maxPrice int64, elasticity float64) *PricingEngine {
	return &PricingEngine{
		basePrice:  basePrice,
		minPrice:   minPrice,
		maxPrice:   maxPrice,
		elasticity: elasticity,
	}
}

// PricingFactors 结构体包含了影响商品价格的各种实时因素。
// 这些因素通常由外部系统（如库存系统、用户行为分析系统）提供。
type PricingFactors struct {
	Stock           int32   // 当前可用库存量。
	TotalStock      int32   // 总库存量。
	DemandLevel     float64 // 当前需求水平（通常在0到1之间，0.5表示平均需求）。
	CompetitorPrice int64   // 竞争对手的同类商品价格（单位：分）。
	TimeOfDay       int     // 当前时间的小时数（0-23）。
	DayOfWeek       int     // 当前星期的天数（0-6，例如0代表星期日）。
	IsHoliday       bool    // 是否是节假日。
	UserLevel       int     // 当前用户的等级（例如，1-10，VIP用户等级更高）。
	SeasonFactor    float64 // 季节性因素（通常在0到1之间，0.5表示平均季节影响）。
}

// PricingResult 结构体包含价格计算结果及各因素的影响详情。
type PricingResult struct {
	FinalPrice       int64   // 最终价格
	BasePrice        int64   // 基础价格
	InventoryFactor  float64 // 库存因素调整系数
	DemandFactor     float64 // 需求因素调整系数
	CompetitorFactor float64 // 竞品因素调整系数
	TimeFactor       float64 // 时间因素调整系数
	SeasonFactor     float64 // 季节因素调整系数
	UserFactor       float64 // 用户因素调整系数
}

// CalculatePrice 根据一系列动态定价因素计算商品的调整后价格。
// 这个方法综合了库存、需求、竞品价格、时间、日期、节假日、用户等级和季节等因素。
// 返回详细的计算结果。
func (pe *PricingEngine) CalculatePrice(factors PricingFactors) PricingResult {
	price := float64(pe.basePrice) // 从基础价格开始调整。
	result := PricingResult{
		BasePrice:        pe.basePrice,
		InventoryFactor:  1.0,
		DemandFactor:     1.0,
		CompetitorFactor: 1.0,
		TimeFactor:       1.0,
		SeasonFactor:     1.0,
		UserFactor:       1.0,
	}

	// 1. 库存因素：库存量越少，价格越高；库存量越多，价格越低。
	// 避免除以零。
	stockRatio := 0.0
	if factors.TotalStock > 0 {
		stockRatio = float64(factors.Stock) / float64(factors.TotalStock)
	}

	if stockRatio < 0.1 { // 库存不足10%，价格上涨20%。
		result.InventoryFactor = 1.2
	} else if stockRatio < 0.3 { // 库存不足30%，价格上涨10%。
		result.InventoryFactor = 1.1
	} else if stockRatio > 0.8 { // 库存超过80%，价格下降10%。
		result.InventoryFactor = 0.9
	}
	price *= result.InventoryFactor

	// 2. 需求因素：需求水平越高，价格越高。
	// (factors.DemandLevel - 0.5) * 0.4 会在需求水平为0.5时为0，需求高时为正，需求低时为负。
	result.DemandFactor = 1.0 + (factors.DemandLevel-0.5)*0.4
	price *= result.DemandFactor

	// 3. 竞品价格因素：根据与竞品价格的比较调整自身价格。
	if factors.CompetitorPrice > 0 {
		competitorRatio := float64(pe.basePrice) / float64(factors.CompetitorPrice)
		if competitorRatio > 1.1 { // 如果我们的基础价格比竞品高10%以上，则适当降价。
			result.CompetitorFactor = 0.95
		} else if competitorRatio < 0.9 { // 如果我们的基础价格比竞品低10%以上，则可以适当涨价。
			result.CompetitorFactor = 1.05
		}
	}
	price *= result.CompetitorFactor

	// 4. 时间因素：例如，在一天中的高峰时段提高价格。
	if factors.TimeOfDay >= 10 && factors.TimeOfDay <= 22 { // 早上10点到晚上10点，价格上涨5%。
		result.TimeFactor *= 1.05
	}

	// 5. 星期因素：例如，在周末提高价格。
	if factors.DayOfWeek == 0 || factors.DayOfWeek == 6 { // 星期日或星期六，价格上涨8%。
		result.TimeFactor *= 1.08
	}

	// 6. 节假日因素：在节假日提高价格。
	if factors.IsHoliday {
		result.TimeFactor *= 1.15 // 节假日价格上涨15%。
	}
	price *= result.TimeFactor

	// 7. 用户等级因素：根据用户等级给予不同折扣。
	if factors.UserLevel >= 8 { // VIP用户享受9折优惠。
		result.UserFactor = 0.9
	} else if factors.UserLevel >= 5 { // 高级用户享受95折优惠。
		result.UserFactor = 0.95
	}
	price *= result.UserFactor

	// 8. 季节因素：根据季节性波动调整价格。
	// (factors.SeasonFactor - 0.5) * 0.2 会在季节因子为0.5时为0，季节性旺盛时为正，淡季时为负。
	result.SeasonFactor = 1.0 + (factors.SeasonFactor-0.5)*0.2
	price *= result.SeasonFactor

	// 将计算出的浮点价格转换为整数分，并限制在最小和最高价格之间。
	result.FinalPrice = min(max(int64(price), pe.minPrice), pe.maxPrice)

	return result
}

// CalculateDemandElasticity 基于两点计算简单的需求价格弹性。
func (pe *PricingEngine) CalculateDemandElasticity(currentPrice, currentDemand int64, newPrice, newDemand int64) float64 {
	// 计算价格变化率。
	priceChange := (float64(newPrice) - float64(currentPrice)) / float64(currentPrice)
	// 计算需求量变化率。
	demandChange := (float64(newDemand) - float64(currentDemand)) / float64(currentDemand)

	if priceChange == 0 {
		return 0 // 如果价格没有变化，弹性为0。
	}

	return demandChange / priceChange // 弹性系数 = 需求量变化率 / 价格变化率。
}

// EstimateElasticityFromHistory 使用历史数据通过线性回归估算特定价格点的弹性。
// 弹性 e = (dQ/dP) * (P/Q)。 在线性模型 Q = a + bP 中， dQ/dP = b。
func (pe *PricingEngine) EstimateElasticityFromHistory(price int64, historicalData []DemandData) float64 {
	if len(historicalData) < 2 {
		return pe.elasticity // 数据不足，返回默认弹性
	}

	n := float64(len(historicalData))
	var sumX, sumY, sumXY, sumX2 float64

	for _, data := range historicalData {
		x := float64(data.Price)
		y := float64(data.Demand)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// 计算回归系数 b (即 dQ/dP)
	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0
	}
	b := (n*sumXY - sumX*sumY) / denominator
	a := (sumY - b*sumX) / n

	// 计算该价格点的预测需求 Q
	q := a + b*float64(price)
	if q <= 0 {
		return 0
	}

	// 弹性 e = b * (P/Q)
	return b * (float64(price) / q)
}

// OptimalPrice 计算实现利润最大化的最优价格。
// 假设利润 = (价格 - 成本) * 销量，且销量与价格之间存在线性关系。
// 这个简化模型通过求导并令导数为0来找到理论上的最优价格。
// cost: 商品的单位成本（单位：分）。
// baseDemand: 在基础价格下的基础需求量。
// 返回计算出的最优价格（单位：分）。
func (pe *PricingEngine) OptimalPrice(cost int64, baseDemand int64) int64 {
	// 假设销量 Q = baseDemand * (1 + elasticity * (price - basePrice) / basePrice)
	// 利润 P = (price - cost) * Q
	// 对 P 关于 price 求导，并令导数等于0，可以得到最优价格的表达式。
	// 公式推导简化后：OptimalPrice = (cost * elasticity * basePrice - basePrice * baseDemand) / (2 * elasticity * basePrice - baseDemand)

	numerator := float64(cost)*pe.elasticity*float64(pe.basePrice) - float64(pe.basePrice)*float64(baseDemand)
	denominator := 2*pe.elasticity*float64(pe.basePrice) - float64(baseDemand)

	if denominator == 0 {
		return pe.basePrice // 避免除以零，返回基础价格。
	}

	optimalPrice := min(
		// 限制最优价格在最小和最高价格之间。
		max(int64(numerator/denominator), pe.minPrice), pe.maxPrice)

	return optimalPrice
}

// SurgePrice 实现高峰定价策略（类似Uber的动态定价）。
// 价格根据需求与供给的比率动态调整，比率越高价格越高。
// demandSupplyRatio: 需求与供给的比率。
// 返回调整后的价格（单位：分）。
func (pe *PricingEngine) SurgePrice(demandSupplyRatio float64) int64 {
	var multiplier float64 // 价格乘数。

	if demandSupplyRatio < 0.5 { // 供大于求，降价。
		multiplier = 0.8
	} else if demandSupplyRatio < 1.0 { // 供需平衡。
		multiplier = 1.0
	} else if demandSupplyRatio < 2.0 { // 需求略高。
		multiplier = 1.0 + (demandSupplyRatio-1.0)*0.5
	} else if demandSupplyRatio < 5.0 { // 需求较高。
		multiplier = 1.5 + (demandSupplyRatio-2.0)*0.3
	} else { // 需求极高。
		multiplier = 2.4 // 最大涨价140%。
	}

	price := min(
		// 限制价格范围。
		max(int64(float64(pe.basePrice)*multiplier), pe.minPrice), pe.maxPrice)

	return price
}

// TimeBasedPrice 实现基于时间的定价策略（早鸟价、尾货价等）。
// 价格根据商品在销售周期中所处的时间段进行调整。
// startTime, endTime: 销售活动的开始和结束时间。
// currentTime: 当前时间。
// 返回调整后的价格（单位：分）。
func (pe *PricingEngine) TimeBasedPrice(startTime, endTime, currentTime time.Time) int64 {
	totalDuration := endTime.Sub(startTime).Seconds() // 销售总时长。
	elapsed := currentTime.Sub(startTime).Seconds()   // 已过去的时长。

	// 确保已过去时长在合理范围内。
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > totalDuration {
		elapsed = totalDuration
	}

	progress := elapsed / totalDuration // 销售进度（0-1）。

	var multiplier float64
	if progress < 0.2 { // 销售周期前20%，早鸟价，8折。
		multiplier = 0.8
	} else if progress < 0.5 { // 销售周期20%-50%，正常价。
		multiplier = 1.0
	} else if progress < 0.8 { // 销售周期50%-80%，略微涨价。
		multiplier = 1.1
	} else { // 销售周期最后20%，尾货价，7折清仓。
		multiplier = 0.7
	}

	price := min(
		// 限制价格范围。
		max(int64(float64(pe.basePrice)*multiplier), pe.minPrice), pe.maxPrice)

	return price
}

// PersonalizedPrice 实现个性化定价策略。
// 根据用户的个人画像（如购买力、价格敏感度、忠诚度等）调整商品价格。
// userProfile: 用户的画像信息。
// 返回调整后的价格（单位：分）。
func (pe *PricingEngine) PersonalizedPrice(userProfile UserProfile) int64 {
	price := float64(pe.basePrice)

	// 1. 购买力因素：购买力强的用户可能能接受更高价格。
	if userProfile.PurchasePower > 8 {
		price *= 1.1 // 高购买力用户，价格略高。
	} else if userProfile.PurchasePower < 3 {
		price *= 0.9 // 低购买力用户，价格略低。
	}

	// 2. 价格敏感度：价格敏感的用户给予折扣。
	if userProfile.PriceSensitivity > 7 {
		price *= 0.95 // 价格敏感用户，给予折扣。
	}

	// 3. 忠诚度：高忠诚度用户给予VIP折扣。
	if userProfile.Loyalty > 8 {
		price *= 0.92 // 高忠诚度用户，VIP折扣。
	}

	// 4. 购买频率：高购买频率用户给予折扣。
	if userProfile.PurchaseFrequency > 10 {
		price *= 0.95 // 高频用户，给予折扣。
	}

	// 5. 客单价：高客单价用户给予折扣。
	if userProfile.AvgOrderValue > 50000 {
		price *= 0.93 // 高客单价用户，给予折扣。
	}

	finalPrice := min(
		// 限制价格范围。
		max(int64(price), pe.minPrice), pe.maxPrice)

	return finalPrice
}

// UserProfile 结构体定义了用于个性化定价的用户画像信息。
// 注意：此结构体通常在外部定义，此处为占位。
type UserProfile struct {
	PurchasePower     int   // 购买力（例如，1-10分，分数越高购买力越强）。
	PriceSensitivity  int   // 价格敏感度（例如，1-10分，分数越高越敏感）。
	Loyalty           int   // 忠诚度（例如，1-10分，分数越高越忠诚）。
	PurchaseFrequency int   // 购买频率（例如，每月购买次数）。
	AvgOrderValue     int64 // 平均客单价（单位：分）。
}

// BundlePrice 实现捆绑定价策略。
// 将多个商品打包销售，并给予捆绑折扣。
// items: 捆绑商品的原价列表（单位：分）。
// bundleDiscount: 捆绑折扣率（例如，0.1表示打9折）。
// 返回捆绑销售后的总价格（单位：分）。
func (pe *PricingEngine) BundlePrice(items []int64, bundleDiscount float64) int64 {
	totalPrice := int64(0)
	for _, price := range items {
		totalPrice += price // 计算所有商品原价的总和。
	}

	// 应用捆绑折扣。
	bundlePrice := int64(float64(totalPrice) * (1.0 - bundleDiscount))

	return bundlePrice
}

// CompetitivePricing 实现竞争定价策略。
// 根据竞争对手的价格和预设的策略（如最低价、平均价、溢价）来调整自身价格。
// competitorPrices: 竞争对手的同类商品价格列表（单位：分）。
// strategy: 竞争定价策略，例如 "lowest", "average", "premium"。
// 返回调整后的价格（单位：分）。
func (pe *PricingEngine) CompetitivePricing(competitorPrices []int64, strategy string) int64 {
	if len(competitorPrices) == 0 {
		return pe.basePrice // 没有竞品价格，返回基础价格。
	}

	var price int64

	switch strategy {
	case "lowest":
		// 最低价策略：比竞品中最低价再低一点。
		price = competitorPrices[0]
		for _, p := range competitorPrices {
			if p < price {
				price = p
			}
		}
		price = int64(float64(price) * 0.95) // 比最低价再低5%。

	case "average":
		// 平均价策略：取竞品平均价格。
		sum := int64(0)
		for _, p := range competitorPrices {
			sum += p
		}
		price = sum / int64(len(competitorPrices))

	case "premium":
		// 溢价策略：比竞品平均价格高一些。
		sum := int64(0)
		for _, p := range competitorPrices {
			sum += p
		}
		avgPrice := sum / int64(len(competitorPrices))
		price = int64(float64(avgPrice) * 1.1) // 比平均价高10%。

	default:
		price = pe.basePrice // 未知策略，返回基础价格。
	}

	// 限制价格范围。
	if price < pe.minPrice {
		price = pe.minPrice
	}
	if price > pe.maxPrice {
		price = pe.maxPrice
	}

	return price
}

// PredictDemand 预测在给定价格下的商品需求量。
// 此处使用简化的线性回归模型进行预测。
// price: 待预测价格（单位：分）。
// historicalData: 历史价格和需求量数据。
// 返回预测的需求量。
func (pe *PricingEngine) PredictDemand(price int64, historicalData []DemandData) int64 {
	if len(historicalData) == 0 {
		return 0 // 没有历史数据，无法预测。
	}

	// 使用简单的线性回归模型预测：y = a + b*x，其中 y=需求量, x=价格。
	// 计算回归系数 a 和 b。

	n := float64(len(historicalData))
	var sumX, sumY, sumXY, sumX2 float64

	for _, data := range historicalData {
		x := float64(data.Price)
		y := float64(data.Demand)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// 计算回归系数 b。
	// b = (n*Sum(XY) - Sum(X)*Sum(Y)) / (n*Sum(X^2) - Sum(X)^2)
	b := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	// 计算回归系数 a。
	// a = (Sum(Y) - b*Sum(X)) / n
	a := (sumY - b*sumX) / n

	// 预测需求。
	predictedDemand := a + b*float64(price)

	// 需求量不能为负。
	if predictedDemand < 0 {
		predictedDemand = 0
	}

	return int64(predictedDemand)
}

// DemandData 结构体定义了历史需求数据点。
// 注意：此结构体通常在外部定义，此处为占位。
type DemandData struct {
	Price  int64 // 价格
	Demand int64 // 需求量
}

// CalculateRevenue 计算总收入。
// price: 商品的单价。
// demand: 销售的需求量。
// 返回总收入。
func (pe *PricingEngine) CalculateRevenue(price, demand int64) int64 {
	return price * demand
}

// CalculateProfit 计算总利润。
// price: 商品的单价。
// cost: 商品的单位成本。
// demand: 销售的需求量。
// 返回总利润。
func (pe *PricingEngine) CalculateProfit(price, cost, demand int64) int64 {
	return (price - cost) * demand
}

// OptimalPriceForProfit 使用黄金分割法搜索利润最大化的最优价格。
// cost: 商品的单位成本。
// demandFunc: 一个函数，根据价格预测需求量。
// 返回能够使利润最大化的价格。
func (pe *PricingEngine) OptimalPriceForProfit(cost int64, demandFunc func(int64) int64) int64 {
	// 黄金分割法是一种用于在一维区间内搜索函数极值点的优化算法。
	// 它的核心思想是通过不断缩小搜索区间来逼近最优解。
	goldenRatio := (math.Sqrt(5) - 1) / 2

	left := float64(pe.minPrice)
	right := float64(pe.maxPrice)

	// 当搜索区间足够小（例如，小于1）时停止。
	for right-left > 1 {
		mid1 := left + (right-left)*(1-goldenRatio)
		mid2 := left + (right-left)*goldenRatio

		// 计算在两个中间点对应的利润。
		profit1 := pe.CalculateProfit(int64(mid1), cost, demandFunc(int64(mid1)))
		profit2 := pe.CalculateProfit(int64(mid2), cost, demandFunc(int64(mid2)))

		// 根据利润大小调整搜索区间。
		if profit1 > profit2 {
			right = mid2
		} else {
			left = mid1
		}
	}

	// 返回搜索区间中点的价格作为最优价格。
	return int64((left + right) / 2)
}
