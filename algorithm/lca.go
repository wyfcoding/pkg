// Package algorithm 提供了高性能的图论算法实现。
package algorithm

// TreeLCA 实现了基于倍增（Binary Lifting）算法的最近公共祖先查询。
// 预处理复杂度 O(N log N)，单次查询复杂度 O(log N)。
type TreeLCA struct {
	up    []int
	depth []int
	logN  int
}

// NewTreeLCA 构造一个新的 LCA 查询实例。
func NewTreeLCA(root int, adj [][]int) *TreeLCA {
	n := len(adj)
	if n == 0 {
		return nil
	}

	// 计算最大跳数的对数.
	logN := 1
	for (1 << uint32(logN)) < n {
		logN++
	}

	lca := &TreeLCA{
		up:    make([]int, n*logN),
		depth: make([]int, n),
		logN:  logN,
	}

	// 预初始化 up 数组，默认父节点为自己.
	for i := range lca.up {
		lca.up[i] = -1
	}

	// 深度优先遍历建立倍增表.
	lca.iterativeDFS(root, adj)

	// 构建倍增表核心逻辑.
	for i := 1; i < logN; i++ {
		for v := range n {
			mid := lca.up[v*logN+i-1]
			if mid != -1 {
				lca.up[v*logN+i] = lca.up[mid*logN+i-1]
			}
		}
	}

	return lca
}

type stackItem struct {
	v, p, d int
}

func (lca *TreeLCA) iterativeDFS(root int, adj [][]int) {
	stack := []stackItem{{v: root, p: -1, d: 0}}

	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		v, p, d := curr.v, curr.p, curr.d
		lca.depth[v] = d
		lca.up[v*lca.logN] = p

		for _, u := range adj[v] {
			stack = append(stack, stackItem{v: u, p: v, d: d + 1})
		}
	}
}

// GetLCA 查询两个节点的最近公共祖先。
func (lca *TreeLCA) GetLCA(u, v int) int {
	if lca.depth[u] < lca.depth[v] {
		u, v = v, u
	}

	// 1. 将 u 提升到与 v 同一深度。
	diff := lca.depth[u] - lca.depth[v]
	for i := range lca.logN {
		shift := uint32(i & 0x1F)
		if (diff & (1 << shift)) != 0 {
			u = lca.up[u*lca.logN+i]
		}
	}

	if u == v {
		return u
	}

	// 2. 同时提升 u 和 v，直到它们的父节点相同。
	for i := lca.logN - 1; i >= 0; i-- {
		idxU := u*lca.logN + i
		idxV := v*lca.logN + i
		if lca.up[idxU] != lca.up[idxV] {
			u = lca.up[idxU]
			v = lca.up[idxV]
		}
	}

	return lca.up[u*lca.logN]
}
