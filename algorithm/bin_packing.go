package algorithm

import (
	"sort"
)

// Item 代表需要装箱的物品
type Item struct {
	ID     string
	Volume float64
}

// Bin 代表一个包装箱
type Bin struct {
	ID        int
	Capacity  float64
	Remaining float64
	Items     []Item
}

// BinPackingOptimizer 装箱优化器
type BinPackingOptimizer struct {
	binCapacity float64
}

func NewBinPackingOptimizer(binCapacity float64) *BinPackingOptimizer {
	return &BinPackingOptimizer{binCapacity: binCapacity}
}

// FFD (First Fit Decreasing) 算法
// 1. 将物品按体积从大到小排序
// 2. 遍历物品，寻找第一个能放下它的箱子
// 3. 如果所有现有箱子都放不下，开一个新箱子
func (o *BinPackingOptimizer) FFD(items []Item) []*Bin {
	if len(items) == 0 {
		return nil
	}

	// 1. 复制并排序
	sortedItems := make([]Item, len(items))
	copy(sortedItems, items)
	sort.Slice(sortedItems, func(i, j int) bool {
		return sortedItems[i].Volume > sortedItems[j].Volume
	})

	bins := make([]*Bin, 0)

	// 2. 装箱过程
	for _, item := range sortedItems {
		placed := false
		// 尝试放入已有箱子
		for _, bin := range bins {
			if bin.Remaining >= item.Volume {
				bin.Items = append(bin.Items, item)
				bin.Remaining -= item.Volume
				placed = true
				break
			}
		}

		// 3. 放不下，开新箱子
		if !placed {
			newBin := &Bin{
				ID:        len(bins) + 1,
				Capacity:  o.binCapacity,
				Remaining: o.binCapacity - item.Volume,
				Items:     []Item{item},
			}
			bins = append(bins, newBin)
		}
	}

	return bins
}
