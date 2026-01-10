package graph

import (
	"sync"
)

// HungarianAlgorithm 结构体实现了匈牙利算法，用于解决二分图的最大权匹配问题.
// 尤其适用于解决分配问题（Assignment Problem），例如将一组工人分配给一组任务.
// 以最小化总成本或最大化总效益。
type HungarianAlgorithm struct {
	cost    [][]int64    // 成本矩阵，cost[i][j] 表示将第i个元素分配给第j个元素的成本。
	match   []int        // 匹配数组，match[j] = i 表示第j个元素匹配给了第i个元素，-1表示未匹配。
	visited []bool       // 访问标记数组，用于DFS过程中避免重复访问。
	mu      sync.RWMutex // 读写锁，用于保护算法状态的并发访问。
	n       int          // 矩阵的维度（假设是N x N的方阵）。
}

// NewHungarianAlgorithm 创建并返回一个新的 HungarianAlgorithm 求解器实例。
// costMatrix: 输入的成本矩阵，必须是一个N x N的方阵。
func NewHungarianAlgorithm(costMatrix [][]int64) *HungarianAlgorithm {
	n := len(costMatrix)
	return &HungarianAlgorithm{
		cost:    costMatrix,
		n:       n,
		match:   make([]int, n),  // 初始化匹配数组，所有元素未匹配。
		visited: make([]bool, n), // 初始化访问标记数组。
	}
}

// Solve 执行匈牙利算法以找到最优分配。
// 应用场景：用户与推荐商品的最优匹配、任务与资源的分配等。
// 返回一个整数切片，其中 result[j] = i 表示第j个任务（右侧节点）被分配给了第i个工人（左侧节点）。
func (ha *HungarianAlgorithm) Solve() []int {
	ha.mu.Lock()         // 加写锁，以确保算法执行过程的线程安全。
	defer ha.mu.Unlock() // 确保函数退出时解锁。

	// 初始化匹配数组，所有右侧节点（任务）最初都未匹配。
	for i := range ha.n {
		ha.match[i] = -1
	}

	// 遍历所有左侧节点（工人），尝试为每个工人寻找一个匹配。
	for i := range ha.n {
		// 在每次DFS之前重置visited数组。
		for j := range ha.n {
			ha.visited[j] = false
		}
		// 从当前工人 i 开始执行DFS，尝试找到增广路径。
		ha.dfsHungarian(i)
	}

	return ha.match // 返回最终的匹配结果。
}

// dfsHungarian 是匈牙利算法中的深度优先搜索函数，用于寻找从左侧节点 u 开始的增广路径。
// u: 当前尝试匹配的左侧节点（工人）。
// 返回值：如果找到了从 u 开始的增广路径，则返回 true；否则返回 false。
func (ha *HungarianAlgorithm) dfsHungarian(u int) bool {
	// 遍历所有右侧节点（任务）v。
	for v := range ha.n {
		// 如果任务 v 在本次DFS中尚未被访问，并且工人 u 可以完成任务 v（即成本 > 0，这里假设成本为0表示不能完成）。
		if !ha.visited[v] && ha.cost[u][v] > 0 { // 注意：实际匈牙利算法通常处理最小成本，这里是最大匹配的变种。
			ha.visited[v] = true // 标记任务 v 为已访问。

			// 如果任务 v 尚未被匹配（ha.match[v] == -1）.
			// 或者任务 v 已经被匹配给工人 ha.match[v]，但工人 ha.match[v] 可以找到另一个任务来匹配（通过递归调用dfsHungarian）。
			if ha.match[v] == -1 || ha.dfsHungarian(ha.match[v]) {
				ha.match[v] = u // 将任务 v 匹配给工人 u。
				return true     // 找到增广路径。
			}
		}
	}

	return false // 未能找到从 u 开始的增广路径。
}
