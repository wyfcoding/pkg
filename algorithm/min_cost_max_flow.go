package algorithm

import (
	"math"
	"sync"
)

// MinCostMaxFlowGraph 结构体代表一个最小成本最大流网络图。
// 这种图的每条边除了有容量限制外，还有单位流量的成本。
// 算法目标是在满足最大流的同时，使总成本最小。
type MinCostMaxFlowGraph struct {
	capacity map[string]map[string]int64 // capacity[u][v] 表示从节点u到节点v的边的容量。
	cost     map[string]map[string]int64 // cost[u][v] 表示从节点u到节点v的单位流量成本。
	flow     map[string]map[string]int64 // flow[u][v] 表示从节点u到节点v的当前流量。
	nodes    map[string]bool             // 图中所有节点的集合。
	mu       sync.RWMutex                // 读写锁，用于保护图的并发访问。
}

// NewMinCostMaxFlowGraph 创建并返回一个新的 MinCostMaxFlowGraph 实例。
func NewMinCostMaxFlowGraph() *MinCostMaxFlowGraph {
	return &MinCostMaxFlowGraph{
		capacity: make(map[string]map[string]int64),
		cost:     make(map[string]map[string]int64),
		flow:     make(map[string]map[string]int64),
		nodes:    make(map[string]bool),
	}
}

// AddEdge 向最小成本最大流网络图中添加一条边。
// from: 边的起始节点。
// to: 边的结束节点。
// cap: 边的容量。
// c: 边的单位流量成本。
// 注意：在残余网络中，反向边的成本是负值。
func (g *MinCostMaxFlowGraph) AddEdge(from, to string, cap, c int64) {
	g.mu.Lock()         // 加写锁，以确保修改图的线程安全。
	defer g.mu.Unlock() // 确保函数退出时解锁。

	// 确保起始和结束节点在容量、成本和流量图中都有对应的内层map。
	if g.capacity[from] == nil {
		g.capacity[from] = make(map[string]int64)
		g.cost[from] = make(map[string]int64)
		g.flow[from] = make(map[string]int64)
	}
	if g.capacity[to] == nil {
		g.capacity[to] = make(map[string]int64)
		g.cost[to] = make(map[string]int64)
		g.flow[to] = make(map[string]int64)
	}

	g.capacity[from][to] += cap // 增加正向边的容量（处理并行边）。
	g.cost[from][to] = c        // 设置正向边的成本。
	g.cost[to][from] = -c       // 设置反向边的成本为负值。
	g.nodes[from] = true        // 记录节点。
	g.nodes[to] = true          // 记录节点。
}

// MinCostMaxFlow 算法实现了最小成本最大流问题。
// 它通过反复寻找从源点到汇点的最短增广路径（使用SPFA算法），并沿着该路径增加流量，
// 直到无法再增加流量或达到最大流量限制为止。
// 应用场景：多仓库配送成本最小化、物流路径优化、资源调度等。
// source: 流的源点。
// sink: 流的汇点。
// maxFlow: 需要达到的最大流量目标。
// 返回实际达到的总流量和对应的最小总成本。
func (g *MinCostMaxFlowGraph) MinCostMaxFlow(source, sink string, maxFlow int64) (int64, int64) {
	g.mu.Lock()         // 加写锁，因为会修改流量矩阵和总成本。
	defer g.mu.Unlock() // 确保函数退出时解锁。

	totalFlow := int64(0) // 累计的总流量。
	totalCost := int64(0) // 累计的总成本。

	for totalFlow < maxFlow { // 当总流量未达到最大流量目标时，继续寻找增广路径。
		// 使用SPFA算法在残余网络中寻找一条从源点到汇点的最短（最小成本）增广路径。
		dist, parent := g.spfa(source)

		// 如果无法从源点到达汇点，或者最短路径成本为无穷大，则终止算法。
		if dist[sink] == math.MaxInt64 {
			break
		}

		// 找到当前增广路径上的最小剩余容量（瓶颈容量）。
		pathFlow := maxFlow - totalFlow // 路径流量不能超过还需要发送的流量。
		for v := sink; v != source; v = parent[v] {
			u := parent[v]
			// 剩余容量 = 边的容量 - 边的当前流量。
			residual := g.capacity[u][v] - g.flow[u][v]
			if residual < pathFlow {
				pathFlow = residual
			}
		}

		// 沿着增广路径更新流量和总成本。
		for v := sink; v != source; v = parent[v] {
			u := parent[v]
			g.flow[u][v] += pathFlow             // 正向边增加流量。
			g.flow[v][u] -= pathFlow             // 反向边减少流量。
			totalCost += pathFlow * g.cost[u][v] // 累加成本。
		}

		totalFlow += pathFlow // 将本次增广路径的流量累加到总流量中。
	}

	return totalFlow, totalCost
}

// spfa (Shortest Path Faster Algorithm) 算法用于在有负权边的图中寻找最短路径。
// 在最小成本最大流算法中，SPFA用于在残余网络中寻找最小成本增广路径。
// source: 搜索的起始节点。
// 返回一个 map，其中键是节点，值是从源点到该节点的最小成本；另一个map存储路径中的前驱节点。
func (g *MinCostMaxFlowGraph) spfa(source string) (map[string]int64, map[string]string) {
	dist := make(map[string]int64)    // 存储从源点到各个节点的最短距离。
	parent := make(map[string]string) // 存储路径中的前驱节点。
	inQueue := make(map[string]bool)  // 标记节点是否在队列中，用于优化。

	// 初始化距离：所有节点距离设为无穷大，源点距离设为0。
	for node := range g.nodes {
		dist[node] = math.MaxInt64
	}
	dist[source] = 0

	queue := []string{source} // SPFA队列，初始化为源点。
	inQueue[source] = true    // 标记源点在队列中。

	for len(queue) > 0 {
		u := queue[0]      // 取出队列头节点。
		queue = queue[1:]  // 队列出队。
		inQueue[u] = false // 标记 u 不再队列中。

		// 遍历节点 u 的所有邻接点 v。
		for v := range g.capacity[u] {
			// 只有在残余网络中仍有容量的边才考虑。
			if g.capacity[u][v] > g.flow[u][v] {
				// 尝试通过 u 优化到 v 的距离。
				newDist := dist[u] + g.cost[u][v]
				if newDist < dist[v] {
					dist[v] = newDist // 更新最短距离。
					parent[v] = u     // 更新前驱节点。
					if !inQueue[v] {  // 如果 v 不在队列中，则将其加入队列。
						queue = append(queue, v)
						inQueue[v] = true
					}
				}
			}
		}
	}

	return dist, parent
}
