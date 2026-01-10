package algorithm

import (
	"math"
)

// MCMFEdge 费用流边
type MCMFEdge struct {
	To, Cap, Flow, Cost, Rev int
}

// MCMF 最小费用最大流算法
// 优化：复用内存 buffer，减少 GC 压力
type MCMF struct {
	nodes   int
	adj     [][]MCMFEdge
	dist    []int
	pNode   []int
	pEdge   []int
	inQueue []bool // 复用 buffer
	queue   []int  // 复用 buffer
}

func NewMCMF(n int) *MCMF {
	return &MCMF{
		nodes:   n,
		adj:     make([][]MCMFEdge, n),
		dist:    make([]int, n),
		pNode:   make([]int, n),
		pEdge:   make([]int, n),
		inQueue: make([]bool, n),
		queue:   make([]int, 0, n),
	}
}

func (m *MCMF) AddEdge(u, v, capacity, cost int) {
	m.adj[u] = append(m.adj[u], MCMFEdge{v, capacity, 0, cost, len(m.adj[v])})
	m.adj[v] = append(m.adj[v], MCMFEdge{u, 0, 0, -cost, len(m.adj[u]) - 1})
}

// spfa 寻找最短路径
func (m *MCMF) spfa(s, t int) bool {
	// 重置 dist
	for i := range m.dist {
		m.dist[i] = math.MaxInt32
	}
	// 重置 inQueue (O(N))
	for i := range m.inQueue {
		m.inQueue[i] = false
	}
	// 重置 queue
	m.queue = m.queue[:0]

	m.dist[s] = 0
	m.queue = append(m.queue, s)
	m.inQueue[s] = true

	for len(m.queue) > 0 {
		u := m.queue[0]
		m.queue = m.queue[1:]
		m.inQueue[u] = false

		for i, e := range m.adj[u] {
			if e.Cap > e.Flow && m.dist[e.To] > m.dist[u]+e.Cost {
				m.dist[e.To] = m.dist[u] + e.Cost
				m.pNode[e.To] = u
				m.pEdge[e.To] = i
				if !m.inQueue[e.To] {
					m.queue = append(m.queue, e.To)
					m.inQueue[e.To] = true
				}
			}
		}
	}
	return m.dist[t] != math.MaxInt32
}

// Solve 计算最小费用最大流
// 返回: (最大流, 最小费用)
func (m *MCMF) Solve(s, t int) (int, int) {
	maxFlow, minCost := 0, 0
	for m.spfa(s, t) {
		flow := math.MaxInt32
		for v := t; v != s; v = m.pNode[v] {
			edge := m.adj[m.pNode[v]][m.pEdge[v]]
			if edge.Cap-edge.Flow < flow {
				flow = edge.Cap - edge.Flow
			}
		}
		maxFlow += flow
		for v := t; v != s; v = m.pNode[v] {
			m.adj[m.pNode[v]][m.pEdge[v]].Flow += flow
			revIdx := m.adj[m.pNode[v]][m.pEdge[v]].Rev
			m.adj[v][revIdx].Flow -= flow
			minCost += flow * m.adj[m.pNode[v]][m.pEdge[v]].Cost
		}
	}
	return maxFlow, minCost
}
