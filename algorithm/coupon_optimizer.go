package algorithm

import (
	"log/slog"
	"sort"
	"time"
)

// CouponType 优惠券的类型。
type CouponType int

const (
	CouponTypeDiscount  CouponType = iota + 1 // 折扣券：按比例减少价格，如8折。
	CouponTypeReduction                       // 满减券：达到一定门槛后减免固定金额。
	CouponTypeCash                            // 立减券：直接减免固定金额，无门槛。
)

// Coupon 结构体定义了优惠券的基本属性。
type Coupon struct {
	ID              uint64     // 优惠券的唯一标识符。
	Type            CouponType // 优惠券类型。
	Threshold       int64      // 门槛金额（单位：分），只有订单价格达到此金额才可使用。
	DiscountRate    float64    // 折扣率（0.0-1.0，例如0.8表示8折），仅适用于折扣券。
	ReductionAmount int64      // 减免金额（单位：分），适用于满减券和立减券。
	MaxDiscount     int64      // 最大优惠金额（单位：分），用于限制折扣券的最大优惠。
	CanStack        bool       // 是否可以与其他优惠券叠加使用。
	Priority        int        // 优惠券的优先级，数字越大优先级越高，用于排序和决策。
}

// CouponOptimizer 优惠券优化器，提供了计算最优优惠组合的算法。
type CouponOptimizer struct{}

// NewCouponOptimizer 创建并返回一个新的 CouponOptimizer 实例。
func NewCouponOptimizer() *CouponOptimizer {
	return &CouponOptimizer{}
}

// OptimalCombination 使用暴力枚举法（通过位运算遍历所有子集）计算给定订单价格和优惠券集合的最优优惠组合。
// 这种方法可以找到全局最优解，但计算复杂度较高，适用于优惠券数量较少的情况。
// originalPrice: 原始订单价格（单位：分）。
// coupons: 可用的优惠券列表。
// 返回值：
//   - []uint64: 最优组合的优惠券ID列表。
//   - int64: 使用最优组合后的最终价格（单位：分）。
//   - int64: 最优组合带来的总优惠金额（单位：分）。
func (co *CouponOptimizer) OptimalCombination(
	originalPrice int64,
	coupons []Coupon,
) ([]uint64, int64, int64) {
	start := time.Now()
	if len(coupons) == 0 {
		return nil, originalPrice, 0
	}

	// 过滤掉不满足门槛条件的优惠券，只保留可用的。
	available := make([]Coupon, 0)
	for _, c := range coupons {
		if originalPrice >= c.Threshold {
			available = append(available, c)
		}
	}

	if len(available) == 0 {
		return nil, originalPrice, 0
	}

	// 按照优先级对可用优惠券进行排序（优先级高的在前）。
	sort.Slice(available, func(i, j int) bool {
		return available[i].Priority > available[j].Priority
	})

	// 初始化最优组合、最低价格和最大优惠金额。
	bestCombination := make([]uint64, 0)
	bestPrice := originalPrice
	maxDiscount := int64(0)

	// 使用位运算枚举所有可能的优惠券组合（子集）。
	// mask 的每一位代表一个优惠券是否被选中。
	n := len(available)
	for mask := 1; mask < (1 << n); mask++ { // 从1开始，排除空组合。
		combination := make([]Coupon, 0)

		// 根据mask的位来构建当前组合。
		for i := range n {
			if mask&(1<<i) != 0 { // 如果第i位是1，则选中第i个优惠券。
				combination = append(combination, available[i])
			}
		}

		// 检查当前优惠券组合是否合法（例如，不可叠加的优惠券是否被叠加）。
		if !co.isValidCombination(combination) {
			continue // 如果不合法，则跳过此组合。
		}

		// 计算该组合的最终价格。
		finalPrice := co.calculatePrice(originalPrice, combination)
		discount := originalPrice - finalPrice

		// 如果当前组合能得到更低的价格，则更新最优解。
		if finalPrice < bestPrice {
			bestPrice = finalPrice
			maxDiscount = discount
			// 记录当前最优组合的优惠券ID。
			bestCombination = make([]uint64, len(combination))
			for i, c := range combination {
				bestCombination[i] = c.ID
			}
		}
	}

	slog.Info("OptimalCombination optimization completed", "original_price", originalPrice, "final_price", bestPrice, "discount", maxDiscount, "duration", time.Since(start))
	return bestCombination, bestPrice, maxDiscount
}

// GreedyOptimization 使用贪心算法计算优惠组合。
// 这种方法通常比暴力枚举快，但不能保证找到全局最优解。
// 它优先选择单个优惠金额最大的优惠券，但会在选择过程中考虑叠加规则。
// originalPrice: 原始订单价格（单位：分）。
// coupons: 可用的优惠券列表。
// 返回值：
//   - []uint64: 选定的优惠券ID列表.
//   - int64: 使用选定组合后的最终价格（单位：分）。
//   - int64: 选定组合带来的总优惠金额（单位：分）。
func (co *CouponOptimizer) GreedyOptimization(
	originalPrice int64,
	coupons []Coupon,
) ([]uint64, int64, int64) {
	start := time.Now()
	// 过滤掉不满足门槛条件的优惠券。
	available := make([]Coupon, 0)
	for _, c := range coupons {
		if originalPrice >= c.Threshold {
			available = append(available, c)
		}
	}

	if len(available) == 0 {
		return nil, originalPrice, 0
	}

	// 计算每个优惠券单独使用时的优惠金额，并按照优惠金额降序排序。
	type couponDiscount struct {
		coupon   Coupon
		discount int64
	}

	discounts := make([]couponDiscount, 0)
	for _, c := range available {
		discount := co.calculateSingleDiscount(originalPrice, c)
		discounts = append(discounts, couponDiscount{c, discount})
	}

	// 按照单个优惠金额从大到小排序。
	sort.Slice(discounts, func(i, j int) bool {
		return discounts[i].discount > discounts[j].discount
	})

	// 贪心选择过程：
	selected := make([]Coupon, 0) // 存储已选择的优惠券。
	currentPrice := originalPrice // 当前价格，初始为原始价格。

	for _, cd := range discounts {
		// 尝试添加当前优惠券到已选择的列表中。
		testCombination := append(selected, cd.coupon)

		// 检查加入此优惠券后的组合是否合法。
		if co.isValidCombination(testCombination) {
			// 如果合法，计算新组合后的价格。
			newPrice := co.calculatePrice(originalPrice, testCombination)
			// 如果新价格更低，则接受此优惠券。
			if newPrice < currentPrice {
				selected = testCombination
				currentPrice = newPrice
			}
		}
	}

	// 提取最终选定的优惠券ID。
	result := make([]uint64, len(selected))
	for i, c := range selected {
		result[i] = c.ID
	}

	discount := originalPrice - currentPrice // 计算总优惠金额。
	slog.Info("GreedyOptimization optimization completed", "original_price", originalPrice, "final_price", currentPrice, "discount", discount, "duration", time.Since(start))
	return result, currentPrice, discount
}

// isValidCombination 检查给定的优惠券组合是否合法。
// 当前的合法性规则是：如果组合中有多张优惠券，则所有优惠券都必须是可叠加的。
func (co *CouponOptimizer) isValidCombination(coupons []Coupon) bool {
	if len(coupons) == 0 {
		return false
	}

	if len(coupons) == 1 {
		return true // 单张优惠券总是合法的。
	}

	// 如果有多张优惠券，检查它们是否都可叠加。
	for _, c := range coupons {
		if !c.CanStack {
			return false // 只要有一张不可叠加，则整个组合不合法。
		}
	}

	return true // 所有优惠券都可叠加，组合合法。
}

// calculatePrice 计算使用给定优惠券组合后的最终价格。
// 它会根据优惠券的类型和优先级，顺序应用优惠。
// originalPrice: 原始订单价格。
// coupons: 要应用的优惠券组合。
func (co *CouponOptimizer) calculatePrice(originalPrice int64, coupons []Coupon) int64 {
	if len(coupons) == 0 {
		return originalPrice
	}

	// 复制优惠券列表并按照预设规则排序，以确保正确的优惠应用顺序。
	// 例如：折扣券优先于满减券，以获得最大优惠。
	sorted := make([]Coupon, len(coupons))
	copy(sorted, coupons)

	sort.Slice(sorted, func(i, j int) bool {
		// 先按优惠券类型排序（折扣券 < 满减券 < 立减券），确保特定类型优先应用。
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type < sorted[j].Type
		}
		// 同类型优惠券，按优先级降序排列。
		return sorted[i].Priority > sorted[j].Priority
	})

	currentPrice := originalPrice // 从原始价格开始计算。

	for _, c := range sorted {
		switch c.Type {
		case CouponTypeDiscount:
			// 折扣券：计算折扣金额。
			discount := int64(float64(currentPrice) * (1 - c.DiscountRate))
			// 限制最大优惠金额。
			if c.MaxDiscount > 0 && discount > c.MaxDiscount {
				discount = c.MaxDiscount
			}
			currentPrice -= discount

		case CouponTypeReduction:
			// 满减券：如果满足门槛，则减免。
			if currentPrice >= c.Threshold {
				currentPrice -= c.ReductionAmount
			}

		case CouponTypeCash:
			// 立减券：直接减免。
			currentPrice -= c.ReductionAmount
		}

		// 确保价格不会低于0。
		if currentPrice < 0 {
			currentPrice = 0
		}
	}

	return currentPrice
}

// calculateSingleDiscount 计算单个优惠券对给定原始价格产生的优惠金额。
// originalPrice: 原始订单价格。
// coupon: 待计算的优惠券。
func (co *CouponOptimizer) calculateSingleDiscount(originalPrice int64, coupon Coupon) int64 {
	switch coupon.Type {
	case CouponTypeDiscount:
		discount := int64(float64(originalPrice) * (1 - coupon.DiscountRate))
		if coupon.MaxDiscount > 0 && discount > coupon.MaxDiscount {
			discount = coupon.MaxDiscount
		}
		return discount

	case CouponTypeReduction:
		if originalPrice >= coupon.Threshold {
			return coupon.ReductionAmount
		}
		return 0

	case CouponTypeCash:
		return coupon.ReductionAmount

	default:
		return 0 // 未知优惠券类型返回0优惠。
	}
}

// DynamicProgramming 使用动态规划算法计算最优优惠组合。
// 此方法适合处理优惠券数量较多，且需要找到全局最优解的场景。
// originalPrice: 原始订单价格（单位：分）。
// coupons: 可用的优惠券列表。
// 返回值：
//   - []uint64: 最优组合的优惠券ID列表。
//   - int64: 使用最优组合后的最终价格（单位：分）。
//   - int64: 最优组合带来的总优惠金额（单位：分）。
//
// 注意：此处的动态规划实现可能存在简化，完整最优解通常需要更复杂的DP状态设计。
func (co *CouponOptimizer) DynamicProgramming(
	originalPrice int64,
	coupons []Coupon,
) ([]uint64, int64, int64) {
	// 过滤掉不满足门槛条件的优惠券。
	available := make([]Coupon, 0)
	for _, c := range coupons {
		if originalPrice >= c.Threshold {
			available = append(available, c)
		}
	}

	if len(available) == 0 {
		return nil, originalPrice, 0
	}

	n := len(available)

	// dp[i][0] 表示不选择第i个优惠券时的最低价格。
	// dp[i][1] 表示选择第i个优惠券时的最低价格。
	dp := make([][]int64, n+1)
	for i := range dp {
		dp[i] = make([]int64, 2)
		dp[i][0] = originalPrice
		dp[i][1] = originalPrice
	}

	// choice[i][1] 标记是否选择第i个优惠券。
	choice := make([][]bool, n+1)
	for i := range choice {
		choice[i] = make([]bool, 2)
	}

	for i := 1; i <= n; i++ {
		coupon := available[i-1]

		// 情况1: 不选择第i个优惠券。
		// 最低价格与选择前i-1个优惠券且不选第i-1个时的价格相同。
		dp[i][0] = dp[i-1][0]

		// 情况2: 选择第i个优惠券。
		if coupon.CanStack {
			// 如果优惠券可叠加，则在之前已优惠的基础上继续应用。
			// 假设dp[i-1][1]是选择前i-1个优惠券的最低价格。
			testPrice := co.calculatePrice(dp[i-1][1], []Coupon{coupon})
			if testPrice < dp[i-1][1] {
				dp[i][1] = testPrice
				choice[i][1] = true // 标记选择此优惠券。
			} else {
				dp[i][1] = dp[i-1][1]
			}
		} else {
			// 如果优惠券不可叠加，则单独计算此优惠券应用后的价格。
			dp[i][1] = co.calculatePrice(originalPrice, []Coupon{coupon})
			choice[i][1] = true // 标记选择此优惠券。
		}

		// 更新不选择第i个优惠券的情况，以确保dp[i][0]存储的是前i个优惠券的最低价格。
		// 比较选择和不选择第i个优惠券后的最低价格。
		if dp[i][1] < dp[i][0] {
			dp[i][0] = dp[i][1]
		}
	}

	// 回溯找出被选择的优惠券。
	selected := make([]uint64, 0)
	minPrice := min(
		// 最终的最低价格。
		// 考虑最后一步选择或不选择第n个优惠券。
		dp[n][1], dp[n][0])

	// 从后往前遍历DP表，根据 choice 数组和最终价格回溯路径。
	for i := n; i > 0; i-- {
		// 如果第i个优惠券被选中，并且它导致了当前的最低价格。
		if choice[i][1] && dp[i][1] == minPrice {
			selected = append(selected, available[i-1].ID) // 添加到结果列表。
			// 更新minPrice为前一个状态的最低价格，继续回溯。
			minPrice = dp[i-1][1] // 这个回溯逻辑需要根据实际DP状态定义调整。
		}
	}

	discount := originalPrice - minPrice
	return selected, minPrice, discount
}
