package algorithm

// PSTNode 主席树节点。
type PSTNode struct {
	L, R  int // 左右子节点索引。
	Count int // 该区间内的元素个数 (或权重和)。
}

// PersistentSegmentTree 可持久化线段树 (主席树)。
// 适用于：历史版本查询、区间第 K 大查询。
type PersistentSegmentTree struct {
	roots []int     // 每个版本的根节点索引。
	nodes []PSTNode // 静态数组模拟动态节点。
	n     int       // 区间上限 [1, n]。
}

// NewPersistentSegmentTree 创建一个新的主席树。
// maxOp: 预估的操作次数，用于分配初始内存。
func NewPersistentSegmentTree(n, maxOp int) *PersistentSegmentTree {
	// 空间复杂度 O(M log N)。
	expectedNodes := maxOp * 40
	return &PersistentSegmentTree{
		roots: make([]int, 0),
		nodes: make([]PSTNode, 1, expectedNodes), // 0 号节点作为空节点。
		n:     n,
	}
}

// build 构建初始空树 (全 0)。
func (t *PersistentSegmentTree) build(l, r int) int {
	idx := len(t.nodes)
	t.nodes = append(t.nodes, PSTNode{})
	if l < r {
		mid := (l + r) >> 1
		t.nodes[idx].L = t.build(l, mid)
		t.nodes[idx].R = t.build(mid+1, r)
	}
	return idx
}

// Update 在旧版本 root 基础上，将 pos 位置的值 +1，产生新版本并返回新根。
func (t *PersistentSegmentTree) update(prevRoot, l, r, pos int) int {
	idx := len(t.nodes)
	t.nodes = append(t.nodes, t.nodes[prevRoot]) // 复制旧节点。
	t.nodes[idx].Count++

	if l < r {
		mid := (l + r) >> 1
		if pos <= mid {
			t.nodes[idx].L = t.update(t.nodes[prevRoot].L, l, mid, pos)
		} else {
			t.nodes[idx].R = t.update(t.nodes[prevRoot].R, mid+1, r, pos)
		}
	}
	return idx
}

// PushVersion 基于最新版本添加一个值，产生新版本。
func (t *PersistentSegmentTree) PushVersion(pos int) {
	var prev int
	if len(t.roots) > 0 {
		prev = t.roots[len(t.roots)-1]
	} else {
		prev = t.build(1, t.n)
	}
	newRoot := t.update(prev, 1, t.n, pos)
	t.roots = append(t.roots, newRoot)
}

// QueryRange 查询版本 v 中 [ql, qr] 区间的总和。
func (t *PersistentSegmentTree) QueryRange(vIdx, ql, qr int) int {
	if vIdx < 0 || vIdx >= len(t.roots) {
		return 0
	}
	return t.query(t.roots[vIdx], 1, t.n, ql, qr)
}

func (t *PersistentSegmentTree) query(nodeIdx, l, r, ql, qr int) int {
	if nodeIdx == 0 || ql > r || qr < l {
		return 0
	}
	if ql <= l && r <= qr {
		return t.nodes[nodeIdx].Count
	}
	mid := (l + r) >> 1
	return t.query(t.nodes[nodeIdx].L, l, mid, ql, qr) +
		t.query(t.nodes[nodeIdx].R, mid+1, r, ql, qr)
}

// CurrentVersion 获取当前最新版本号。
func (t *PersistentSegmentTree) CurrentVersion() int {
	return len(t.roots) - 1
}
