package algorithm

import (
	"math"
	"sort"
)

// WarehouseInfo 结构体定义了仓库的基本信息和能力。
type WarehouseInfo struct {
	ID       uint64  // 仓库的唯一标识符。
	Lat      float64 // 仓库的地理纬度。
	Lon      float64 // 仓库的地理经度。
	Stock    int32   // 该仓库中某个SKU的当前库存量。
	Priority int     // 仓库的优先级，例如，主仓库可能优先级更高。
	ShipCost int64   // 从该仓库发货的单位配送成本（单位：分）。
}

// OrderItem 结构体定义了订单中的一个商品项。
type OrderItem struct {
	SkuID    uint64 // 商品SKU的唯一标识符。
	Quantity int32  // 订单中该SKU的需求数量。
}

// AllocationResult 结构体表示一个订单商品分配到特定仓库的结果。
type AllocationResult struct {
	WarehouseID uint64      // 分配到的仓库ID。
	Items       []OrderItem // 分配到该仓库的商品列表（可能只包含部分需求）。
	TotalCost   int64       // 从该仓库发货的总成本。
	Distance    float64     // 用户到该仓库的距离。
}

// WarehouseAllocator 结构体提供了多种策略来优化订单商品的仓库分配。
type WarehouseAllocator struct{}

// NewWarehouseAllocator 创建并返回一个新的 WarehouseAllocator 实例。
func NewWarehouseAllocator() *WarehouseAllocator {
	return &WarehouseAllocator{}
}

// AllocateOptimal 采用综合评分策略进行最优仓库分配。
// 策略：优先选择距离用户最近、库存充足且配送成本较低的仓库。
// 如果一个仓库无法满足所有商品需求，它会尝试分配其能满足的部分，剩余需求将由其他仓库分配。
// userLat, userLon: 用户的地理位置（纬度、经度）。
// items: 订单中所有商品的列表。
// warehouses: 可用的仓库信息，结构为 WarehouseID -> SkuID -> WarehouseInfo。
// 返回一个 AllocationResult 列表，表示每个仓库分配了哪些商品以及总成本。
func (wa *WarehouseAllocator) AllocateOptimal(
	userLat, userLon float64,
	items []OrderItem,
	warehouses map[uint64]map[uint64]*WarehouseInfo, // 仓库ID -> SKU ID -> 仓库信息。
) []AllocationResult {
	results := make([]AllocationResult, 0)
	remainingItems := make(map[uint64]int32) // 存储订单中尚未完全分配的商品及数量。

	// 初始化剩余商品需求。
	for _, item := range items {
		remainingItems[item.SkuID] = item.Quantity
	}

	// 计算每个仓库的综合评分。
	type warehouseScore struct {
		warehouseID uint64
		distance    float64 // 用户到仓库的距离。
		totalStock  int32   // 该仓库对当前订单商品的可用总库存。
		avgCost     int64   // 从该仓库发货的平均成本。
		score       float64 // 综合评分。
	}

	scores := make([]warehouseScore, 0)

	// 遍历所有仓库，计算它们的评分。
	for warehouseID, skuMap := range warehouses {
		if len(skuMap) == 0 {
			continue // 跳过空仓库。
		}

		// 获取仓库的地理位置信息（假设同一仓库的所有SKU共享位置信息）。
		var warehouseInfo *WarehouseInfo
		for _, info := range skuMap { // 从任意一个SKU中获取仓库信息。
			warehouseInfo = info
			break
		}

		// 计算用户到仓库的距离。
		distance := haversineDistance(userLat, userLon, warehouseInfo.Lat, warehouseInfo.Lon)

		// 计算该仓库对当前订单的可用总库存和平均配送成本。
		var totalStock int32
		var totalCost int64
		var count int // 统计该仓库有库存的SKU数量。

		for skuID := range remainingItems { // 只计算订单中需要的SKU。
			if info, exists := skuMap[skuID]; exists {
				totalStock += info.Stock
				totalCost += info.ShipCost
				count++
			}
		}

		avgCost := int64(0)
		if count > 0 {
			avgCost = totalCost / int64(count) // 计算平均成本。
		}

		// 综合评分：权重分配示例（距离权重0.4，库存权重0.3，成本权重0.3）。
		// 距离越近分数越高，库存越多分数越高，成本越低分数越高。
		distanceScore := 1.0 / (1.0 + distance/1000.0)    // 距离归一化：距离越小，得分越大。
		stockScore := float64(totalStock) / 1000.0        // 库存归一化：库存越大，得分越大。
		costScore := 1.0 / (1.0 + float64(avgCost)/100.0) // 成本归一化：成本越小，得分越大。

		score := 0.4*distanceScore + 0.3*stockScore + 0.3*costScore // 计算综合评分。

		scores = append(scores, warehouseScore{
			warehouseID: warehouseID,
			distance:    distance,
			totalStock:  totalStock,
			avgCost:     avgCost,
			score:       score,
		})
	}

	// 按照综合评分降序排序仓库，优先处理评分高的仓库。
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// 贪心分配：从评分最高的仓库开始分配商品。
	for _, ws := range scores {
		if len(remainingItems) == 0 {
			break // 所有商品都已分配完毕。
		}

		warehouseID := ws.warehouseID
		skuMap := warehouses[warehouseID] // 获取当前仓库的SKU信息。

		allocatedItems := make([]OrderItem, 0) // 记录当前仓库分配的商品。
		totalCost := int64(0)                  // 记录当前仓库的总配送成本。

		for skuID, needQty := range remainingItems {
			// 如果该仓库有此SKU的库存。
			if info, exists := skuMap[skuID]; exists && info.Stock > 0 {
				allocQty := min(
					// 默认分配所需数量。
					info.Stock,
					// 如果仓库库存不足，则分配所有可用库存。
					needQty)

				// 记录分配给当前仓库的商品。
				allocatedItems = append(allocatedItems, OrderItem{
					SkuID:    skuID,
					Quantity: allocQty,
				})

				totalCost += info.ShipCost * int64(allocQty) // 累加配送成本。

				// 更新剩余需求。
				remainingItems[skuID] -= allocQty
				if remainingItems[skuID] <= 0 {
					delete(remainingItems, skuID) // 如果该SKU的需求已完全满足，则从剩余需求中移除。
				}
			}
		}

		if len(allocatedItems) > 0 {
			results = append(results, AllocationResult{
				WarehouseID: warehouseID,
				Items:       allocatedItems,
				TotalCost:   totalCost,
				Distance:    ws.distance,
			})
		}
	}

	return results
}

// AllocateByDistance 按距离分配策略。
// 优先选择距离用户最近的仓库进行分配。
// userLat, userLon: 用户的地理位置（纬度、经度）。
// items: 订单中所有商品的列表。
// warehouses: 可用的仓库信息。
// 返回 AllocationResult 列表。
func (wa *WarehouseAllocator) AllocateByDistance(
	userLat, userLon float64,
	items []OrderItem,
	warehouses map[uint64]map[uint64]*WarehouseInfo,
) []AllocationResult {
	type warehouseDist struct {
		warehouseID uint64
		distance    float64
	}

	distances := make([]warehouseDist, 0)

	// 计算所有仓库到用户的距离。
	for warehouseID, skuMap := range warehouses {
		if len(skuMap) == 0 {
			continue
		}

		var warehouseInfo *WarehouseInfo
		for _, info := range skuMap {
			warehouseInfo = info
			break
		}

		distance := haversineDistance(userLat, userLon, warehouseInfo.Lat, warehouseInfo.Lon)
		distances = append(distances, warehouseDist{warehouseID, distance})
	}

	// 按照距离升序排序仓库，距离越近越优先。
	sort.Slice(distances, func(i, j int) bool {
		return distances[i].distance < distances[j].distance
	})

	results := make([]AllocationResult, 0)
	remainingItems := make(map[uint64]int32)

	for _, item := range items {
		remainingItems[item.SkuID] = item.Quantity
	}

	// 遍历按距离排序后的仓库，进行分配。
	for _, wd := range distances {
		if len(remainingItems) == 0 {
			break // 所有商品都已分配完毕。
		}

		warehouseID := wd.warehouseID
		skuMap := warehouses[warehouseID]

		allocatedItems := make([]OrderItem, 0)
		totalCost := int64(0)

		for skuID, needQty := range remainingItems {
			if info, exists := skuMap[skuID]; exists && info.Stock > 0 {
				allocQty := min(info.Stock, needQty)

				allocatedItems = append(allocatedItems, OrderItem{
					SkuID:    skuID,
					Quantity: allocQty,
				})

				totalCost += info.ShipCost * int64(allocQty)

				remainingItems[skuID] -= allocQty
				if remainingItems[skuID] <= 0 {
					delete(remainingItems, skuID)
				}
			}
		}

		if len(allocatedItems) > 0 {
			results = append(results, AllocationResult{
				WarehouseID: warehouseID,
				Items:       allocatedItems,
				TotalCost:   totalCost,
				Distance:    wd.distance,
			})
		}
	}

	return results
}

// haversineDistance 计算两个地理坐标点之间的Haversine距离。
// lat1, lon1: 第一个点的纬度、经度。
// lat2, lon2: 第二个点的纬度、经度。
// 返回两个点之间的距离（单位：米）。
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0 // 地球平均半径（米）。

	// 将角度转换为弧度。
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	// Haversine公式计算球面距离。
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}
