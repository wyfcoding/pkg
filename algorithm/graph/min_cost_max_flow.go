package graph

import (
	"math"
	"sync"
)

// MinCostMaxFlowGraph 结构体代表一个最小成本最大流网络图。
type MinCostMaxFlowGraph struct {
	nodeID map[string]int // 字符串节点到整数 ID 的映射
	adj    [][]mcmfEdge   // 邻接表
	mu     sync.RWMutex   // 线程安全
}

type mcmfEdge struct {
	to   int
	rev  int   // 反向边在 adj[to] 中的索引
	cap  int64 // 容量
	cost int64 // 单位流量成本
	flow int64 // 当前流量
}

// NewMinCostMaxFlowGraph 创建一个预分配节点空间的 MinCostMaxFlowGraph 实例。
func NewMinCostMaxFlowGraph(n int) *MinCostMaxFlowGraph {
	return &MinCostMaxFlowGraph{
		nodeID: make(map[string]int),
		adj:    make([][]mcmfEdge, n),
	}
}

// getID 获取或创建节点的整数 ID。
func (g *MinCostMaxFlowGraph) getID(name string) int {
	if id, ok := g.nodeID[name]; ok {
		return id
	}
	id := len(g.adj)
	g.nodeID[name] = id
	g.adj = append(g.adj, make([]mcmfEdge, 0))
	return id
}

// AddEdge 向图中添加一条有向边（使用字符串标识）。
func (g *MinCostMaxFlowGraph) AddEdge(from, to string, capacity, cost int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	u := g.getID(from)
	v := g.getID(to)

	g.adj[u] = append(g.adj[u], mcmfEdge{
		to:   v,
		rev:  len(g.adj[v]),
		cap:  capacity,
		cost: cost,
		flow: 0,
	})

	g.adj[v] = append(g.adj[v], mcmfEdge{
		to:   u,
		rev:  len(g.adj[u]) - 1,
		cap:  0,
		cost: -cost,
		flow: 0,
	})
}

// AddEdgeByID 直接使用整数 ID 添加边（高性能场景）。
func (g *MinCostMaxFlowGraph) AddEdgeByID(u, v int, capacity, cost int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	maxID := max(u, v)
	for len(g.adj) <= maxID {
		g.adj = append(g.adj, make([]mcmfEdge, 0))
	}

	g.adj[u] = append(g.adj[u], mcmfEdge{
		to:   v,
		rev:  len(g.adj[v]),
		cap:  capacity,
		cost: cost,
		flow: 0,
	})

	g.adj[v] = append(g.adj[v], mcmfEdge{
		to:   u,
		rev:  len(g.adj[u]) - 1,
		cap:  0,
		cost: -cost,
		flow: 0,
	})
}

// spfa 寻找最短增广路。
func (g *MinCostMaxFlowGraph) spfa(sID, tID int, dist []int64, parent, parentEdge []int) bool {
	n := len(g.adj)
	inQueue := make([]bool, n)
	queue := make([]int, 0, n)

	for i := range n {
		dist[i] = math.MaxInt64
	}

	dist[sID] = 0
	queue = append(queue, sID)
	inQueue[sID] = true

	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		inQueue[u] = false

		for i, e := range g.adj[u] {
			if e.cap > e.flow && dist[e.to] > dist[u]+e.cost {
				dist[e.to] = dist[u] + e.cost
				parent[e.to] = u
				parentEdge[e.to] = i
				if !inQueue[e.to] {
					queue = append(queue, e.to)
					inQueue[e.to] = true
				}
			}
		}
	}

	return dist[tID] != math.MaxInt64
}

// Solve 执行最小成本最大流算法（使用字符串标识）。
func (g *MinCostMaxFlowGraph) Solve(source, sink string, maxFlow int64) (flow, cost int64) {
	g.mu.Lock()
	sID, ok1 := g.nodeID[source]
	tID, ok2 := g.nodeID[sink]
	g.mu.Unlock()

	if !ok1 || !ok2 {
		return 0, 0
	}
	return g.SolveByID(sID, tID, maxFlow)
}

// SolveByID 直接通过 ID 计算最小成本最大流。
func (g *MinCostMaxFlowGraph) SolveByID(sID, tID int, maxFlow int64) (flow, cost int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var totalFlow, totalCost int64
	n := len(g.adj)
	dist := make([]int64, n)
	parentEdge := make([]int, n)
	parent := make([]int, n)

	for totalFlow < maxFlow {
		if !g.spfa(sID, tID, dist, parent, parentEdge) {
			break
		}

		pathFlow := maxFlow - totalFlow
		for curr := tID; curr != sID; curr = parent[curr] {
			p := parent[curr]
			idx := parentEdge[curr]
			if f := g.adj[p][idx].cap - g.adj[p][idx].flow; f < pathFlow {
				pathFlow = f
			}
		}

		for curr := tID; curr != sID; curr = parent[curr] {
			p := parent[curr]
			idx := parentEdge[curr]
			g.adj[p][idx].flow += pathFlow
			totalCost += pathFlow * g.adj[p][idx].cost
			g.adj[curr][g.adj[p][idx].rev].flow -= pathFlow
		}
		totalFlow += pathFlow
	}

	return totalFlow, totalCost
}
