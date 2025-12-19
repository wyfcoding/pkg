package algorithm

import (
	"sync"
)

// SegmentTree (线段树) 是一种树状数据结构，用于高效地处理数组或序列的区间查询和单点更新操作。
// 它的每个节点都代表着数组的一个区间，根节点代表整个数组，叶子节点代表数组中的单个元素。
// 更新和查询操作的时间复杂度均为 O(log N)。
// 在实际应用中，例如库存管理（查询某个商品分类的总库存）、销量统计（查询某段时间内的总销量）等场景中非常有用。
type SegmentTree struct {
	tree []int64      // 存储线段树的节点值。通常需要 4*N 的空间来构建树。
	n    int          // 原始数组的逻辑大小。
	mu   sync.RWMutex // 读写锁，用于保护线段树的并发访问。
}

// NewSegmentTree 创建并返回一个新的 SegmentTree 实例。
// n: 原始数组的逻辑大小。
func NewSegmentTree(n int) *SegmentTree {
	return &SegmentTree{
		tree: make([]int64, 4*n), // 通常线段树需要 4N 大小的存储空间。
		n:    n,
	}
}

// Update 单点更新原始数组中指定索引处的元素，并相应地更新线段树。
// idx: 原始数组中要更新的元素的0-indexed索引。
// val: 元素的新值。
func (st *SegmentTree) Update(idx int, val int64) {
	st.mu.Lock()         // 加写锁，以确保更新操作的线程安全。
	defer st.mu.Unlock() // 确保函数退出时解锁。

	// 从根节点（索引1）开始递归更新。
	st.update(1, 0, st.n-1, idx, val)
}

// update 是 Update 的递归辅助函数，用于实际更新线段树节点。
// node: 当前线段树节点的索引。
// start, end: 当前节点所代表区间的起始和结束索引（在原始数组中）。
// idx: 原始数组中要更新的元素的索引。
// val: 元素的新值。
func (st *SegmentTree) update(node, start, end, idx int, val int64) {
	// 如果达到叶子节点，即当前节点代表的区间是一个单点，直接更新其值。
	if start == end {
		st.tree[node] = val
		return
	}

	mid := (start + end) / 2 // 计算当前区间的中间点。
	// 根据 idx 决定进入左子树还是右子树。
	if idx <= mid {
		st.update(2*node, start, mid, idx, val) // 更新左子树。
	} else {
		st.update(2*node+1, mid+1, end, idx, val) // 更新右子树。
	}

	// 更新当前节点的值，通常是其左右子节点值的聚合（例如求和）。
	st.tree[node] = st.tree[2*node] + st.tree[2*node+1]
}

// Query 区间查询原始数组中指定范围 [left, right] 的和。
// 应用场景：例如，查询某个商品分类的总库存，或某段时间内的总销量。
// left, right: 区间查询的起始和结束0-indexed索引。
// 返回区间 [left, right] 的所有元素的和。
func (st *SegmentTree) Query(left, right int) int64 {
	st.mu.RLock()         // 加读锁，以确保查询操作的线程安全。
	defer st.mu.RUnlock() // 确保函数退出时解锁。

	// 从根节点（索引1）开始递归查询。
	return st.query(1, 0, st.n-1, left, right)
}

// query 是 Query 的递归辅助函数，用于实际查询线段树节点。
// node: 当前线段树节点的索引。
// start, end: 当前节点所代表区间的起始和结束索引（在原始数组中）。
// left, right: 目标查询区间的起始和结束索引。
func (st *SegmentTree) query(node, start, end, left, right int) int64 {
	// 情况1: 当前节点代表的区间与目标查询区间完全不重叠。
	if right < start || end < left {
		return 0 // 返回0（对于求和操作）。
	}

	// 情况2: 当前节点代表的区间完全包含在目标查询区间内。
	if left <= start && end <= right {
		return st.tree[node] // 直接返回当前节点的值。
	}

	// 情况3: 当前节点代表的区间与目标查询区间部分重叠，需要递归查询子节点。
	mid := (start + end) / 2 // 计算当前区间的中间点。
	// 递归查询左子树。
	leftSum := st.query(2*node, start, mid, left, right)
	// 递归查询右子树。
	rightSum := st.query(2*node+1, mid+1, end, left, right)

	return leftSum + rightSum // 返回左右子树查询结果的聚合。
}
