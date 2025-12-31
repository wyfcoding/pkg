package algorithm

import (
	"math"
)

// LCANode 代表树中的节点信息
type LCANode struct {
	ID       int
	ParentID int
}

// TreeLCA 实现了倍增法求最近公共祖先
type TreeLCA struct {
	maxNodes int
	logN     int
	up       [][]int // up[i][j] 表示节点 i 的第 2^j 个祖先
	depth    []int
	adj      [][]int
}

func NewTreeLCA(n int, nodes []LCANode) *TreeLCA {
	logN := int(math.Log2(float64(n))) + 1
	lca := &TreeLCA{
		maxNodes: n,
		logN:     logN,
		up:       make([][]int, n),
		depth:    make([]int, n),
		adj:      make([][]int, n),
	}

	for i := range lca.up {
		lca.up[i] = make([]int, logN)
	}

	root := -1
	for _, node := range nodes {
		if node.ParentID == -1 {
			root = node.ID
		} else {
			lca.adj[node.ParentID] = append(lca.adj[node.ParentID], node.ID)
		}
	}

	if root != -1 {
		lca.dfs(root, root, 0)
	}

	return lca
}

func (lca *TreeLCA) dfs(v, p, d int) {
	lca.depth[v] = d
	lca.up[v][0] = p
	for i := 1; i < lca.logN; i++ {
		lca.up[v][i] = lca.up[lca.up[v][i-1]][i-1]
	}
	for _, u := range lca.adj[v] {
		if u != p {
			lca.dfs(u, v, d+1)
		}
	}
}

// GetLCA 查询两个节点的最近公共祖先
func (lca *TreeLCA) GetLCA(u, v int) int {
	if lca.depth[u] < lca.depth[v] {
		u, v = v, u
	}

	// 1. 将 u 提升到与 v 同一深度
	for i := lca.logN - 1; i >= 0; i-- {
		if lca.depth[u]-(1<<uint(i)) >= lca.depth[v] {
			u = lca.up[u][i]
		}
	}

	if u == v {
		return u
	}

	// 2. 同时提升 u 和 v，直到它们的父节点相同
	for i := lca.logN - 1; i >= 0; i-- {
		if lca.up[u][i] != lca.up[v][i] {
			u = lca.up[u][i]
			v = lca.up[v][i]
		}
	}

	return lca.up[u][0]
}

// GetDistance 计算两个节点之间的路径长度
func (lca *TreeLCA) GetDistance(u, v int) int {
	ancestor := lca.GetLCA(u, v)
	return lca.depth[u] + lca.depth[v] - 2*lca.depth[ancestor]
}
