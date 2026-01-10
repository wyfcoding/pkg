package algorithm

import (
	"math"
	"sync"
)

// MinCostMaxFlowGraph 结构体代表一个最小成本最大流网络图。
// 优化：内部使用整数 ID 和邻接表代替字符串 Map，显著提升性能。
type MinCostMaxFlowGraph struct {
	nodeID map[string]int // 字符串节点到整数 ID 的映.
	adj    [][]mcmfEdge   // 邻接.
	mu     sync.RWMutex   // 读写.
}

type mcmfEdge struct {
	to   int
	rev  int   // 反向边在 adj[to] 中的索.
	cap  int64 // 容.
	cost int64 // 单位流量成.
	flow int64 // 当前流.
}

// NewMinCostMaxFlowGraph 创建并返回一个新的 MinCostMaxFlowGraph 实例。
func NewMinCostMaxFlowGraph() *MinCostMaxFlowGraph {
	return &MinCostMaxFlowGraph{
		nodeID: make(map[string]int),
		adj:    make([][]mcmfEdge, 0),
	}
}

// getID 获取或创建节点的整数 I.
func (g *MinCostMaxFlowGraph) getID(name string) int {
	if id, ok := g.nodeID[name]; ok {
		return id
	}
	id := len(g.adj)
	g.nodeID[name] = id
	g.adj = append(g.adj, make([]mcmfEdge, 0))
	return id
}

// AddEdge 向最小成本最大流网络图中添加一条边。
func (g *MinCostMaxFlowGraph) AddEdge(from, to string, capacity, c int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	u := g.getID(from)
	v := g.getID(to)

	// 添加正向.
	g.adj[u] = append(g.adj[u], mcmfEdge{
		to:   v,
		rev:  len(g.adj[v]), // 反向边将是 v 的下一个.
		cap:  capacity,
		cost: c,
		flow: 0,
	})

	// 添加反向边 (容量 0，成本 -c.
	g.adj[v] = append(g.adj[v], mcmfEdge{
		to:   u,
		rev:  len(g.adj[u]) - 1, // 正向边是 u 的最后一个.
		cap:  0,
		cost: -c,
		flow: 0,
	})
}

// spfa 寻找最短增广路.
func (g *MinCostMaxFlowGraph) spfa(sID, tID int, dist []int64, parent []int, parentEdge []int) bool {
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

// MinCostMaxFlow 算法实现了最小成本最大流问题。
func (g *MinCostMaxFlowGraph) MinCostMaxFlow(source, sink string, maxFlow int64) (int64, int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	sID, ok1 := g.nodeID[source]
	tID, ok2 := g.nodeID[sink]
	if !ok1 || !ok2 {
		return 0, 0
	}

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
			if f := g.adj[parent[curr]][parentEdge[curr]].cap - g.adj[parent[curr]][parentEdge[curr]].flow; f < pathFlow {
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
