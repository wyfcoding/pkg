package algorithm

import (
	"sync"
)

// BipartiteGraph 结构体代表一个二分图。
// 它用于解决两组实体之间（例如，配送员和订单）的最佳匹配问题。
type BipartiteGraph struct {
	// edges 存储二分图的边。键是左侧节点（例如，配送员的ID），值是它能连接的右侧节点列表（例如，订单的ID列表）。
	edges map[string][]string
	// match 存储当前的匹配结果。键是右侧节点（订单），值是与它匹配的左侧节点（配送员）。
	match map[string]string
	mu    sync.RWMutex // 读写锁，用于保护图的并发访问。
}

// NewBipartiteGraph 创建并返回一个新的 BipartiteGraph 实例。
func NewBipartiteGraph() *BipartiteGraph {
	return &BipartiteGraph{
		edges: make(map[string][]string),
		match: make(map[string]string),
	}
}

// AddEdge 向二分图中添加一条边。
// deliveryMan: 左侧节点（如配送员的ID）。
// order: 右侧节点（如订单的ID）。
// 表示该配送员可以配送该订单。
func (bg *BipartiteGraph) AddEdge(deliveryMan, order string) {
	bg.mu.Lock()         // 加写锁，以安全地修改图的边。
	defer bg.mu.Unlock() // 确保函数退出时解锁。

	bg.edges[deliveryMan] = append(bg.edges[deliveryMan], order)
}

// MaxMatching 使用匈牙利算法（基于DFS寻找增广路径）计算二分图的最大匹配。
// 在本应用场景中，它可以找到最多的配送员-订单匹配对。
// 返回一个map，表示每个订单匹配到的配送员（match[orderID] = deliveryManID）。
func (bg *BipartiteGraph) MaxMatching() map[string]string {
	bg.mu.Lock()         // 加写锁，因为会修改匹配结果。
	defer bg.mu.Unlock() // 确保函数退出时解锁。

	result := make(map[string]string) // 存储最终的匹配结果。
	// 遍历所有左侧节点（配送员），尝试为每个配送员找到一个匹配。
	for deliveryMan := range bg.edges {
		visited := make(map[string]bool) // visited 用于DFS过程中标记已访问的右侧节点，防止循环。
		bg.dfs(deliveryMan, visited, result)
	}

	return result
}

// dfs 是匈牙利算法中的深度优先搜索函数，用于寻找从节点 u 开始的增广路径。
// u: 当前尝试匹配的左侧节点（配送员）。
// visited: 在本次DFS调用中已访问过的右侧节点（订单）集合，用于避免重复访问和循环。
// match: 当前的匹配状态。
// 返回值：如果找到了从 u 开始的增广路径，则返回 true；否则返回 false。
func (bg *BipartiteGraph) dfs(u string, visited map[string]bool, match map[string]string) bool {
	// 遍历左侧节点 u 的所有邻接右侧节点 v（即配送员 u 可以配送的所有订单 v）。
	for _, v := range bg.edges[u] {
		// 如果在本次DFS中 v 已经被访问过，则跳过，避免陷入死循环。
		if visited[v] {
			continue
		}
		visited[v] = true // 标记 v 为已访问。

		// 如果右侧节点 v 尚未被匹配（match[v]为空），或者 v 已经被匹配但可以为其匹配的左侧节点找到另一条增广路径。
		// 递归调用 dfs 尝试为 match[v] 找到新的匹配。
		if match[v] == "" || bg.dfs(match[v], visited, match) {
			match[v] = u // 将 v 匹配给 u。
			return true  // 找到增广路径。
		}
	}

	return false // 未能找到从 u 开始的增广路径。
}
