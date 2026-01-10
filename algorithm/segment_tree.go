package algorithm

import (
	"sync"
)

// SegmentTree (线段树) 是一种树状数据结构，用于高效地处理数组或序列的区间查询和单点更新操作。
// 此实现采用非递归（迭代）方式（即 zkw 线段树），相比递归实现具有更小的常数因子和更好的缓存友好性。
// 更新和查询操作的时间复杂度均为 O(log N)。
type SegmentTree struct {
	tree []int64      // 存储线段树的节点值。使用扁平化数组存储。
	n    int          // 原始数组的大小。
	mu   sync.RWMutex // 读写锁，用于保护线段树的并发访问。
}

// NewSegmentTree 创建并返回一个新的 SegmentTree 实例。
// n: 原始数组的逻辑大小。
func NewSegmentTree(n int) *SegmentTree {
	// 迭代式线段树通常需要 2*n 的空间。
	// 数组元素存储在下标 [n, 2n-1] 处。
	// 下标 1 是根节点。
	return &SegmentTree{
		tree: make([]int64, 2*n),
		n:    n,
	}
}

// Update 单点更新原始数组中指定索引处的元素，并相应地更新线段树。
// idx: 原始数组中要更新的元素的0-indexed索引。
// val: 元素的新值。
func (st *SegmentTree) Update(idx int, val int64) {
	st.mu.Lock()
	defer st.mu.Unlock()

	// 定位到叶子节点
	pos := idx + st.n
	st.tree[pos] = val

	// 向上更新父节点
	// pos/2 是父节点索引
	// pos^1 是兄弟节点索引 (如果是左孩子(偶数)，^1得到右孩子(奇数)；反之亦然)
	for pos > 1 {
		// Parent = Left Child + Right Child
		// st.tree[pos] 是当前更新的节点，st.tree[pos^1] 是它的兄弟
		// 父节点索引为 pos >> 1
		st.tree[pos>>1] = st.tree[pos] + st.tree[pos^1]
		pos >>= 1
	}
}

// Query 区间查询原始数组中指定范围 [left, right] 的和。
// left, right: 区间查询的起始和结束0-indexed索引 (包含 boundaries)。
func (st *SegmentTree) Query(left, right int) int64 {
	st.mu.RLock()
	defer st.mu.RUnlock()

	var sum int64
	// 将区间映射到线段树的叶子节点范围
	// 注意：迭代式查询通常使用左闭右开区间 [l, r)，
	// 但本方法参数定义为闭区间 [left, right]，所以 right 需要 +1 后再映射，
	// 或者在循环条件中处理。
	// 这里我们采用标准的 [l, r) 转换：
	l := left + st.n
	r := right + st.n + 1 // 转换为右开区间

	for l < r {
		// 如果 l 是右孩子（奇数），它不能覆盖父节点的整个左半部分，
		// 所以直接加上它自己，然后 l 向右移动一位（即 l++），
		// 这样父节点在下一轮循环计算时就会处理 l 的右兄弟（即父节点的下一个节点的左孩子）。
		if l&1 == 1 {
			sum += st.tree[l]
			l++
		}

		// 如果 r 是右孩子（奇数），r-1 是左孩子。
		// 由于 r 是开区间边界，我们需要加的是 r-1 的值。
		if r&1 == 1 {
			r--
			sum += st.tree[r]
		}

		// 上升到父节点层级
		l >>= 1
		r >>= 1
	}

	return sum
}
