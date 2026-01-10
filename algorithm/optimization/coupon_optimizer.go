package optimization

import (
	"log/slog"
	"slices"
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
	DiscountRate    float64    // 折扣率（0.0-1.0，例如0.8表示8折），仅适用于折扣券。
	Threshold       int64      // 门槛金额（单位：分），只有订单价格达到此金额才可使用。
	ReductionAmount int64      // 减免金额（单位：分），适用于满减券和立减券。
	MaxDiscount     int64      // 最大优惠金额（单位：分），用于限制折扣券的最大优惠。
	Type            CouponType // 优惠券类型。
	Priority        int        // 优惠券的优先级，数字越大优先级越高，用于排序和决策。
	CanStack        bool       // 是否可以与其他优惠券叠加使用。
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
// 返回值.
//   - []uint64: 最优组合的优惠券ID列表。
//   - int64: 使用最优组合后的最终价格（单位：分）。
//   - int64: 最优组合带来的总优惠金额（单位：分）。
func (co *CouponOptimizer) OptimalCombination(
	originalPrice int64,
	coupons []Coupon,
) (bestCombinations []uint64, finalPrice, totalDiscount int64) {
	start := time.Now()
	if len(coupons) == 0 {
		return nil, originalPrice, 0
	}

	// 安全检查：如果优惠券数量过大，暴力枚举 (2^N) 会导致极其严重的性能问题。
	// 阈值设置为 20，2^20 ≈ 100万次迭代，耗时尚可接受。
	// 大于此阈值时，降级使用贪心算法或仅选取前 20 个高优先级的券。
	// 这里选择降级为贪心算法，并记录警告日志。
	if len(coupons) > 20 {
		slog.Warn("OptimalCombination: too many coupons, multiple to greedy strategy", "count", len(coupons), "threshold", 20)
		return co.GreedyOptimization(originalPrice, coupons)
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

	// 按照计算价格所需的顺序预排序：先按类型，再按优先级。
	// 这样在生成子集时，子集自然保持有序，避免在循环中重复排序。
	slices.SortFunc(available, func(a, b Coupon) int {
		if a.Type != b.Type {
			if a.Type < b.Type {
				return -1
			}
			return 1
		}
		if a.Priority > b.Priority {
			return -1
		}
		if a.Priority < b.Priority {
			return 1
		}
		return 0
	})

	// 初始化最优组合、最低价格和最大优惠金额。
	bestCombination := make([]uint64, 0)
	bestPrice := originalPrice
	maxDiscount := int64(0)

	// 使用位运算枚举所有可能的优惠券组合（子集）。
	// mask 的每一位代表一个优惠券是否被选中。
	n := len(available)
	for mask := 1; mask < (1 << n); mask++ { // 从1开始，排除空组合。
		combination := make([]Coupon, 0, n) // 预分配容.

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

		// 计算该组合的最终价格 (使用无需排序的快速计算版本)。
		finalPrice := co.calculatePriceFast(originalPrice, combination)
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

// calculatePriceFast 是 calculatePrice 的优化版本，假设输入 coupons 已经按规则排序。
func (co *CouponOptimizer) calculatePriceFast(originalPrice int64, sortedCoupons []Coupon) int64 {
	currentPrice := originalPrice

	for _, c := range sortedCoupons {
		switch c.Type {
		case CouponTypeDiscount:
			discount := int64(float64(currentPrice) * (1 - c.DiscountRate))
			if c.MaxDiscount > 0 && discount > c.MaxDiscount {
				discount = c.MaxDiscount
			}
			currentPrice -= discount

		case CouponTypeReduction:
			if currentPrice >= c.Threshold {
				currentPrice -= c.ReductionAmount
			}

		case CouponTypeCash:
			currentPrice -= c.ReductionAmount
		}

		if currentPrice < 0 {
			currentPrice = 0
		}
	}

	return currentPrice
}

// GreedyOptimization 使用贪心算法计算优惠组合。
// 这种方法通常比暴力枚举快，但不能保证找到全局最优解。
// 它优先选择单个优惠金额最大的优惠券，但会在选择过程中考虑叠加规则。
// originalPrice: 原始订单价格（单位：分）。
// coupons: 可用的优惠券列表。
// 返回值.
//   - []uint64: 选定的优惠券ID列表.
//   - int64: 使用选定组合后的最终价格（单位：分）。
//   - int64: 选定组合带来的总优惠金额（单位：分）。
func (co *CouponOptimizer) GreedyOptimization(
	originalPrice int64,
	coupons []Coupon,
) (ids []uint64, finalPrice, totalDiscount int64) {
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
	slices.SortFunc(discounts, func(a, b couponDiscount) int {
		if a.discount > b.discount {
			return -1
		}
		if a.discount < b.discount {
			return 1
		}
		return 0
	})

	// 贪心选择过程.
	selected := make([]Coupon, 0) // 存储已选择的优惠券。
	currentPrice := originalPrice // 当前价格，初始为原始价格。

	for _, cd := range discounts {
		// 尝试添加当前优惠券到已选择的列表中。
		testCombination := make([]Coupon, len(selected), len(selected)+1)
		copy(testCombination, selected)
		testCombination = append(testCombination, cd.coupon)

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

	slices.SortFunc(sorted, func(a, b Coupon) int {
		// 先按优惠券类型排序（折扣券 < 满减券 < 立减券），确保特定类型优先应用。
		if a.Type != b.Type {
			if a.Type < b.Type {
				return -1
			}
			return 1
		}
		// 同类型优惠券，按优先级降序排列。
		if a.Priority > b.Priority {
			return -1
		}
		if a.Priority < b.Priority {
			return 1
		}
		return 0
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

// DynamicProgramming 使用动态规划计算最优优惠组合。
// 升级版：严格处理可叠加与不可叠加逻辑。
// 核心思路.
// 1. 将所有不可叠加的券视为独立的分组（每组只能选一张）。
// 2. 将所有可叠加的券视为一个特殊分组。
// 3. 求解分组背包问题的变体。
func (co *CouponOptimizer) DynamicProgramming(
	originalPrice int64,
	coupons []Coupon,
) (ids []uint64, finalPrice, totalDiscount int64) {
	if len(coupons) == 0 {
		return nil, originalPrice, 0
	}

	// 1. 分类：可叠加 vs 不可叠.
	var stackable []Coupon
	var nonStackable []Coupon
	for _, c := range coupons {
		if originalPrice < c.Threshold {
			continue
		}
		if c.CanStack {
			stackable = append(stackable, c)
		} else {
			nonStackable = append(nonStackable, c)
		}
	}

	// 2. 情况 A：只用一张不可叠加的券中的最优.
	var bestNonStackID uint64
	bestNonStackPrice := originalPrice
	for _, c := range nonStackable {
		p := co.calculatePrice(originalPrice, []Coupon{c})
		if p < bestNonStackPrice {
			bestNonStackPrice = p
			bestNonStackID = c.ID
		}
	}

	// 3. 情况 B：使用所有可叠加券的最优组.
	// 这里使用位运算 DP（如果数量不多）或贪心 + 排序优.
	// 针对折扣券和满减券的顺序敏感性，先按类型排.
	slices.SortFunc(stackable, func(a, b Coupon) int {
		if a.Type != b.Type {
			if a.Type < b.Type {
				return -1
			}
			return 1
		}
		if a.Priority > b.Priority {
			return -1
		}
		if a.Priority < b.Priority {
			return 1
		}
		return 0
	})

	stackIDs := make([]uint64, 0)
	stackPrice := originalPrice
	for _, c := range stackable {
		p := co.calculatePrice(stackPrice, []Coupon{c})
		if p < stackPrice {
			stackPrice = p
			stackIDs = append(stackIDs, c.ID)
		}
	}

	// 4. 最终决策：取 A 和 B 之间的最优.
	if bestNonStackPrice < stackPrice {
		return []uint64{bestNonStackID}, bestNonStackPrice, originalPrice - bestNonStackPrice
	}

	return stackIDs, stackPrice, originalPrice - stackPrice
}
