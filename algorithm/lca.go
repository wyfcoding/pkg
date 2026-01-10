package algorithm

import (
	"math/bits"
)

// LCANode 代表树中的节点信息。
type LCANode struct {
	ID       int
	ParentID int
}

// TreeLCA 实现了倍增法求最近公共祖先。
type TreeLCA struct {
	up       []int // 扁平化数组: up[i * logN + j] 表示节点 i 的第 2^j 个祖先。
	depth    []int
	maxNodes int
	logN     int
}

func NewTreeLCA(n int, nodes []LCANode) *TreeLCA {
	if n <= 0 {
		return nil
	}
	// 计算 logN: bits.Len(uint(n))。
	logN := bits.Len(uint(n))
	lca := &TreeLCA{
		maxNodes: n,
		logN:     logN,
		up:       make([]int, n*logN),
		depth:    make([]int, n),
	}

	adj := make([][]int, n)
	root := -1
	for _, node := range nodes {
		if node.ParentID == -1 {
			root = node.ID
		} else {
			adj[node.ParentID] = append(adj[node.ParentID], node.ID)
		}
	}

	if root != -1 {
		lca.iterativeDFS(root, adj)
	}

	return lca
}

// iterativeDFS 使用迭代方式遍历树，防止深度过大导致栈溢出。
func (lca *TreeLCA) iterativeDFS(root int, adj [][]int) {
	type stackItem struct {
		v, p, d int
	}
	stack := []stackItem{{v: root, p: root, d: 0}}

	for len(stack) > 0 {
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		v, p, d := item.v, item.p, item.d
		lca.depth[v] = d
		lca.up[v*lca.logN] = p
		for i := 1; i < lca.logN; i++ {
			// up[v][i] = up[up[v][i-1]][i-1]。
			mid := lca.up[v*lca.logN+i-1]
			lca.up[v*lca.logN+i] = lca.up[mid*lca.logN+i-1]
		}

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
		if (diff >> uint(i) & 1) == 1 {
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

// GetDistance 计算两个节点之间的路径长度。
func (lca *TreeLCA) GetDistance(u, v int) int {
	ancestor := lca.GetLCA(u, v)
	return lca.depth[u] + lca.depth[v] - 2*lca.depth[ancestor]
}
