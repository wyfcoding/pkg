package algorithm

import (
	"math"
	"sync"
)

// MaxFlowGraph 结构体代表一个流网络图。
// 流网络是一种有向图，每条边都有一个容量，并且有一个源点和一个汇点。
// 最大流问题是寻找从源点到汇点可以发送的最大流量。
type MaxFlowGraph struct {
	capacity map[string]map[string]int64 // capacity[u][v] 表示从节点u到节点v的边的容量。
	flow     map[string]map[string]int64 // flow[u][v] 表示从节点u到节点v的当前流量。
	nodes    map[string]bool             // 图中所有节点的集合。
	mu       sync.RWMutex                // 读写锁，用于保护图的并发访问。
}

// NewMaxFlowGraph 创建并返回一个新的 MaxFlowGraph 实例。
func NewMaxFlowGraph() *MaxFlowGraph {
	return &MaxFlowGraph{
		capacity: make(map[string]map[string]int64),
		flow:     make(map[string]map[string]int64),
		nodes:    make(map[string]bool),
	}
}

// AddEdge 向流网络图中添加一条边。
// from: 边的起始节点。
// to: 边的结束节点。
// cap: 边的容量。
func (g *MaxFlowGraph) AddEdge(from, to string, cap int64) {
	g.mu.Lock()         // 加写锁，以确保修改图的线程安全。
	defer g.mu.Unlock() // 确保函数退出时解锁。

	// 确保起始和结束节点在容量和流量图中都有对应的内层map。
	if g.capacity[from] == nil {
		g.capacity[from] = make(map[string]int64)
		g.flow[from] = make(map[string]int64)
	}
	if g.capacity[to] == nil {
		g.capacity[to] = make(map[string]int64)
		g.flow[to] = make(map[string]int64)
	}

	g.capacity[from][to] += cap // 增加边的容量（处理并行边）。
	g.nodes[from] = true        // 记录节点。
	g.nodes[to] = true          // 记录节点。
}

// FordFulkerson 算法实现了福特-富尔克森方法来计算流网络中的最大流。
// 它通过反复寻找从源点到汇点的增广路径并增加流量，直到找不到更多增广路径为止。
// 应用场景：多仓库到多订单的最优库存分配、网络路由、任务分配等。
// source: 流的源点。
// sink: 流的汇点。
// 返回从源点到汇点的最大流量。
func (g *MaxFlowGraph) FordFulkerson(source, sink string) int64 {
	g.mu.Lock()         // 加写锁，因为会修改流量矩阵。
	defer g.mu.Unlock() // 确保函数退出时解锁。

	maxFlow := int64(0) // 记录最大总流量。

	for {
		// 使用BFS（广度优先搜索）在残余网络中寻找一条从源点到汇点的增广路径。
		// parent map存储了路径上每个节点的前驱节点，用于回溯路径。
		parent := g.bfs(source, sink)
		if parent == nil {
			// 如果没有找到增广路径，说明已经达到最大流，算法终止。
			break
		}

		// 找到当前增广路径上的最小剩余容量（瓶颈容量）。
		pathFlow := int64(math.MaxInt64)
		for v := sink; v != source; v = parent[v] {
			u := parent[v]
			// 剩余容量 = 边的容量 - 边的当前流量。
			residual := g.capacity[u][v] - g.flow[u][v]
			if residual < pathFlow {
				pathFlow = residual
			}
		}

		// 沿着增广路径更新流量。
		for v := sink; v != source; v = parent[v] {
			u := parent[v]
			g.flow[u][v] += pathFlow // 正向边增加流量。
			g.flow[v][u] -= pathFlow // 反向边减少流量，模拟流量回退的能力。
		}

		maxFlow += pathFlow // 将本次增广路径的流量累加到总最大流量中。
	}

	return maxFlow
}

// bfs 广度优先搜索算法，用于在残余网络中寻找一条从源点到汇点的路径。
// source: 搜索的起始节点。
// sink: 搜索的目标节点。
// 返回一个 map，其中键是节点，值是其在路径中的前驱节点。如果找不到路径，则返回 nil。
func (g *MaxFlowGraph) bfs(source, sink string) map[string]string {
	visited := make(map[string]bool)  // 记录在本次BFS中已访问的节点。
	parent := make(map[string]string) // 记录路径。
	queue := []string{source}         // BFS队列，初始化为源点。
	visited[source] = true            // 标记源点为已访问。

	for len(queue) > 0 {
		u := queue[0]     // 取出队列头节点。
		queue = queue[1:] // 队列出队。

		if u == sink {
			return parent // 如果到达汇点，则找到了路径。
		}

		// 遍历节点 u 的所有邻接点 v。
		for v := range g.capacity[u] {
			// 如果 v 尚未被访问，并且从 u 到 v 在残余网络中仍有剩余容量。
			if !visited[v] && g.capacity[u][v] > g.flow[u][v] {
				visited[v] = true        // 标记 v 为已访问。
				parent[v] = u            // 记录 v 的前驱节点为 u。
				queue = append(queue, v) // v 入队。
			}
		}
	}

	return nil // 没有找到从源点到汇点的路径。
}
