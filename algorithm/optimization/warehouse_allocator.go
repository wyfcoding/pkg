package optimization

import (
	"slices"

	algomath "github.com/wyfcoding/pkg/algorithm/math"
)

const (
	distScoreScale  = 1000.0
	stockScoreScale = 1000.0
	costScoreScale  = 100.0
	weightDistance  = 0.4
	weightStock     = 0.3
	weightCost      = 0.3
)

// WarehouseInfo 结构体定义了仓库的基本信息和能力.
type WarehouseInfo struct {
	Lat      float64
	Lon      float64
	ID       uint64
	ShipCost int64
	Stock    int32
	Priority int
}

// OrderItem 结构体定义了订单中的一个商品项.
type OrderItem struct {
	SkuID    uint64
	Quantity int32
}

// AllocationResult 结构体表示一个订单商品分配到特定仓库的结果.
type AllocationResult struct {
	Items       []OrderItem
	Distance    float64
	WarehouseID uint64
	TotalCost   int64
}

// WarehouseAllocator 结构体提供了多种策略来优化订单商品的仓库分配.
type WarehouseAllocator struct{}

// NewWarehouseAllocator 创建并返回一个新的 WarehouseAllocator 实例.
func NewWarehouseAllocator() *WarehouseAllocator {
	return &WarehouseAllocator{}
}

type warehouseScore struct {
	score       float64
	distance    float64
	warehouseID uint64
	avgCost     int64
	totalStock  int32
}

// AllocateOptimal 采用综合评分策略进行最优仓库分配.
func (wa *WarehouseAllocator) AllocateOptimal(
	userLat, userLon float64,
	items []OrderItem,
	warehouses map[uint64]map[uint64]*WarehouseInfo,
) []AllocationResult {
	results := make([]AllocationResult, 0)
	remaining := make(map[uint64]int32)

	for _, item := range items {
		remaining[item.SkuID] = item.Quantity
	}

	scores := wa.calculateWarehouseScores(userLat, userLon, remaining, warehouses)

	slices.SortFunc(scores, func(a, b warehouseScore) int {
		if a.score > b.score {
			return -1
		}
		if a.score < b.score {
			return 1
		}

		return 0
	})

	for _, ws := range scores {
		if len(remaining) == 0 {
			break
		}

		res := wa.allocateFromWarehouse(ws.warehouseID, ws.distance, remaining, warehouses)
		if len(res.Items) > 0 {
			results = append(results, res)
		}
	}

	return results
}

func (wa *WarehouseAllocator) calculateWarehouseScores(
	userLat, userLon float64,
	remaining map[uint64]int32,
	warehouses map[uint64]map[uint64]*WarehouseInfo,
) []warehouseScore {
	scores := make([]warehouseScore, 0, len(warehouses))

	for wID, skuMap := range warehouses {
		if len(skuMap) == 0 {
			continue
		}

		var wInfo *WarehouseInfo
		for _, info := range skuMap {
			wInfo = info

			break
		}

		dist := algomath.HaversineDistance(userLat, userLon, wInfo.Lat, wInfo.Lon)
		var totalStock int32
		var totalCost int64
		var count int

		for sID := range remaining {
			if info, exists := skuMap[sID]; exists {
				totalStock += info.Stock
				totalCost += info.ShipCost
				count++
			}
		}

		var avgCost int64
		if count > 0 {
			avgCost = totalCost / int64(count)
		}

		dScore := 1.0 / (1.0 + dist/distScoreScale)
		sScore := float64(totalStock) / stockScoreScale
		cScore := 1.0 / (1.0 + float64(avgCost)/costScoreScale)
		score := weightDistance*dScore + weightStock*sScore + weightCost*cScore

		scores = append(scores, warehouseScore{
			warehouseID: wID,
			distance:    dist,
			totalStock:  totalStock,
			avgCost:     avgCost,
			score:       score,
		})
	}

	return scores
}

func (wa *WarehouseAllocator) allocateFromWarehouse(
	wID uint64,
	dist float64,
	remaining map[uint64]int32,
	warehouses map[uint64]map[uint64]*WarehouseInfo,
) AllocationResult {
	skuMap := warehouses[wID]
	allocated := make([]OrderItem, 0)
	var totalCost int64

	for sID, need := range remaining {
		info, exists := skuMap[sID]
		if !exists || info.Stock <= 0 {
			continue
		}
		qty := min(info.Stock, need)
		allocated = append(allocated, OrderItem{SkuID: sID, Quantity: qty})
		totalCost += info.ShipCost * int64(qty)
		remaining[sID] -= qty
		if remaining[sID] <= 0 {
			delete(remaining, sID)
		}
	}

	return AllocationResult{
		WarehouseID: wID,
		Items:       allocated,
		TotalCost:   totalCost,
		Distance:    dist,
	}
}

// AllocateByDistance 按距离分配策略.
func (wa *WarehouseAllocator) AllocateByDistance(
	userLat, userLon float64,
	items []OrderItem,
	warehouses map[uint64]map[uint64]*WarehouseInfo,
) []AllocationResult {
	type wDist struct {
		wID  uint64
		dist float64
	}

	dists := make([]wDist, 0, len(warehouses))
	for wID, skuMap := range warehouses {
		if len(skuMap) == 0 {
			continue
		}

		var wInfo *WarehouseInfo
		for _, info := range skuMap {
			wInfo = info

			break
		}

		d := algomath.HaversineDistance(userLat, userLon, wInfo.Lat, wInfo.Lon)
		dists = append(dists, wDist{wID: wID, dist: d})
	}

	slices.SortFunc(dists, func(a, b wDist) int {
		if a.dist < b.dist {
			return -1
		}
		if a.dist > b.dist {
			return 1
		}

		return 0
	})

	results := make([]AllocationResult, 0)
	remaining := make(map[uint64]int32)
	for _, item := range items {
		remaining[item.SkuID] = item.Quantity
	}

	for _, wd := range dists {
		if len(remaining) == 0 {
			break
		}

		res := wa.allocateFromWarehouse(wd.wID, wd.dist, remaining, warehouses)
		if len(res.Items) > 0 {
			results = append(results, res)
		}
	}

	return results
}
