package algorithm

import (
	"math"
)

// MCMFEdge 费用流边
type MCMFEdge struct {
	To, Cap, Flow, Cost, Rev int
}

// MCMF 最小费用最大流算法
type MCMF struct {
	nodes int
	adj   [][]MCMFEdge
	dist  []int
	pNode []int
	pEdge []int
}

func NewMCMF(n int) *MCMF {
	return &MCMF{
		nodes: n,
		adj:   make([][]MCMFEdge, n),
		dist:  make([]int, n),
		pNode: make([]int, n),
		pEdge: make([]int, n),
	}
}

func (m *MCMF) AddEdge(u, v, cap, cost int) {
	m.adj[u] = append(m.adj[u], MCMFEdge{v, cap, 0, cost, len(m.adj[v])})
	m.adj[v] = append(m.adj[v], MCMFEdge{u, 0, 0, -cost, len(m.adj[u]) - 1})
}

// spfa 寻找最短路径
func (m *MCMF) spfa(s, t int) bool {
	for i := range m.dist {
		m.dist[i] = math.MaxInt32
	}
	inQueue := make([]bool, m.nodes)
	queue := []int{s}
	m.dist[s] = 0
	inQueue[s] = true

	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		inQueue[u] = false

		for i, e := range m.adj[u] {
			if e.Cap > e.Flow && m.dist[e.To] > m.dist[u]+e.Cost {
				m.dist[e.To] = m.dist[u] + e.Cost
				m.pNode[e.To] = u
				m.pEdge[e.To] = i
				if !inQueue[e.To] {
					queue = append(queue, e.To)
					inQueue[e.To] = true
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
