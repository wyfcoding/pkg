package graph

import (
	"sync"
)

// FenwickTree (树状数组) 是一种数据结构，它能够高效地处理数组元素的更新操作和区间前缀和查询。
// 它的时间复杂度在更新和查询操作上均为 O(log N)。
// 在实际应用中，例如库存管理（快速查询某个仓库区域的总库存）、销量统计等场景中非常有用。
type FenwickTree struct {
	tree []int64      // 存储树状数组的数据，通常为1-indexed。
	mu   sync.RWMutex // 读写锁，用于保护树状数组的并发访问。
	n    int          // 原始数组的大小。
}

// NewFenwickTree 创建并返回一个新的 FenwickTree 实例。
// n: 原始数组的逻辑大小（即 FenwickTree 将要操作的数据范围）。
func NewFenwickTree(n int) *FenwickTree {
	return &FenwickTree{
		tree: make([]int64, n+1), // 树状数组通常是1-indexed，所以需要 n+1 的大小。
		n:    n,
	}
}

// Update 更新原始数组中指定索引处的元素，并相应地更新树状数组。
// idx: 原始数组中要更新的元素的0-indexed索引。
// delta: 要添加到该元素的增量值。
func (ft *FenwickTree) Update(idx int, delta int64) {
	ft.mu.Lock()         // 加写锁，以确保更新操作的线程安全。
	defer ft.mu.Unlock() // 确保函数退出时解锁。

	idx++ // 将0-indexed索引转换为1-indexed，因为树状数组通常是1-indexed。
	// 从 idx 开始向上遍历树状数组，更新所有受影响的节点。
	for idx <= ft.n {
		ft.tree[idx] += delta
		// idx += idx & (-idx) 是获取下一个需要更新的父节点的关键操作。
		// 它会跳到 idx 的最低有效位，并将其加上。
		idx += idx & (-idx)
	}
}

// Query 查询原始数组从0到指定索引的区间前缀和。
// idx: 原始数组中查询范围的结束0-indexed索引。
// 返回从0到idx的所有元素的和。
func (ft *FenwickTree) Query(idx int) int64 {
	ft.mu.RLock()         // 加读锁，以确保查询操作的线程安全。
	defer ft.mu.RUnlock() // 确保函数退出时解锁。

	idx++ // 将0-indexed索引转换为1-indexed。
	var sum int64
	// 从 idx 开始向下遍历树状数组，累加所有受影响的节点。
	for idx > 0 {
		sum += ft.tree[idx]
		// idx -= idx & (-idx) 是获取下一个需要累加的父节点的关键操作。
		// 它会跳到 idx 的最低有效位，并将其减去。
		idx -= idx & (-idx)
	}
	return sum
}

// RangeQuery 查询原始数组中指定区间 [left, right] 的和。
// left, right: 区间查询的起始和结束0-indexed索引。
// 返回区间 [left, right] 的所有元素的和。
func (ft *FenwickTree) RangeQuery(left, right int) int64 {
	// 区间和可以通过两个前缀和的差值计算：Sum[left, right] = Sum[0, right] - Sum[0, left-1]。
	if left == 0 {
		return ft.Query(right) // 如果从0开始，直接查询到right的前缀和。
	}
	return ft.Query(right) - ft.Query(left-1) // 否则计算两个前缀和的差值。
}
