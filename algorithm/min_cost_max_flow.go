package algorithm

import (
	"math"
	"sync"
)

// MinCostMaxFlowGraph 结构体代表一个最小成本最大流网络图。
// 优化：内部使用整数 ID 和邻接表代替字符串 Map，显著提升性能。
type MinCostMaxFlowGraph struct {
	nodeID map[string]int // 字符串节点到整数 ID 的映射
	adj    [][]mcmfEdge   // 邻接表
	mu     sync.RWMutex   // 读写锁
}

type mcmfEdge struct {
	to   int
	rev  int   // 反向边在 adj[to] 中的索引
	cap  int64 // 容量
	cost int64 // 单位流量成本
	flow int64 // 当前流量
}

// NewMinCostMaxFlowGraph 创建并返回一个新的 MinCostMaxFlowGraph 实例。
func NewMinCostMaxFlowGraph() *MinCostMaxFlowGraph {
	return &MinCostMaxFlowGraph{
		nodeID: make(map[string]int),
		adj:    make([][]mcmfEdge, 0),
	}
}

// getID 获取或创建节点的整数 ID
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

	// 添加正向边
	g.adj[u] = append(g.adj[u], mcmfEdge{
		to:   v,
		rev:  len(g.adj[v]), // 反向边将是 v 的下一个边
		cap:  capacity,
		cost: c,
		flow: 0,
	})

	// 添加反向边 (容量 0，成本 -c)
	g.adj[v] = append(g.adj[v], mcmfEdge{
		to:   u,
		rev:  len(g.adj[u]) - 1, // 正向边是 u 的最后一个边
		cap:  0,
		cost: -c,
		flow: 0,
	})
}

// MinCostMaxFlow 算法实现了最小成本最大流问题。
func (g *MinCostMaxFlowGraph) MinCostMaxFlow(source, sink string, maxFlow int64) (int64, int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 检查源点和汇点是否存在
	sID, ok1 := g.nodeID[source]
	tID, ok2 := g.nodeID[sink]
	if !ok1 || !ok2 {
		return 0, 0
	}

	totalFlow := int64(0)
	totalCost := int64(0)
	n := len(g.adj)

	// SPFA 辅助数组 (避免每次分配，但这里简单起见每次分配，或者可以像 mcmf.go 那样优化复用)
	// 鉴于图大小不定，且 MinCostMaxFlow 调用频率可能不高，直接分配即可。
	// 若追求极致，可作为成员变量缓存。
	dist := make([]int64, n)
	parentEdge := make([]int, n) // 记录前驱边在 adj[parent[v]] 中的索引
	parent := make([]int, n)     // 记录前驱节点
	inQueue := make([]bool, n)
	queue := make([]int, 0, n)

	for totalFlow < maxFlow {
		// --- SPFA ---
		for i := 0; i < n; i++ {
			dist[i] = math.MaxInt64
			inQueue[i] = false
		}
		queue = queue[:0]

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

		if dist[tID] == math.MaxInt64 {
			break
		}

		// 寻找瓶颈容量
		pathFlow := maxFlow - totalFlow
		curr := tID
		for curr != sID {
			p := parent[curr]
			idx := parentEdge[curr]
			e := g.adj[p][idx]
			if e.cap-e.flow < pathFlow {
				pathFlow = e.cap - e.flow
			}
			curr = p
		}

		// 更新流量
		curr = tID
		for curr != sID {
			p := parent[curr]
			idx := parentEdge[curr]

			// 正向边
			g.adj[p][idx].flow += pathFlow
			totalCost += pathFlow * g.adj[p][idx].cost

			// 反向边
			revIdx := g.adj[p][idx].rev
			g.adj[curr][revIdx].flow -= pathFlow

			curr = p
		}

		totalFlow += pathFlow
	}

	return totalFlow, totalCost
}
