package algorithm

import (
	"math"
)

// Graph 图结构
type Graph struct {
	nodes int
	adj   [][]int
}

func NewGraph(nodes int) *Graph {
	return &Graph{
		nodes: nodes,
		adj:   make([][]int, nodes),
	}
}

func (g *Graph) AddEdge(u, v int) {
	g.adj[u] = append(g.adj[u], v)
}

// TarjanSCC Tarjan 强连通分量算法
type TarjanSCC struct {
	graph   *Graph
	index   int
	stack   []int
	onStack []bool
	indices []int
	lowlink []int
	sccs    [][]int
}

func NewTarjanSCC(g *Graph) *TarjanSCC {
	return &TarjanSCC{
		graph:   g,
		indices: make([]int, g.nodes),
		lowlink: make([]int, g.nodes),
		onStack: make([]bool, g.nodes),
		index:   -1, // 初始化为 -1
	}
}

func (t *TarjanSCC) Run() [][]int {
	for i := range t.indices {
		t.indices[i] = -1
	}

	for i := 0; i < t.graph.nodes; i++ {
		if t.indices[i] == -1 {
			t.strongConnect(i)
		}
	}
	return t.sccs
}

func (t *TarjanSCC) strongConnect(v int) {
	t.index++
	t.indices[v] = t.index
	t.lowlink[v] = t.index
	t.stack = append(t.stack, v)
	t.onStack[v] = true

	for _, w := range t.graph.adj[v] {
		if t.indices[w] == -1 {
			t.strongConnect(w)
			t.lowlink[v] = int(math.Min(float64(t.lowlink[v]), float64(t.lowlink[w])))
		} else if t.onStack[w] {
			t.lowlink[v] = int(math.Min(float64(t.lowlink[v]), float64(t.indices[w])))
		}
	}

	if t.lowlink[v] == t.indices[v] {
		var scc []int
		for {
			w := t.stack[len(t.stack)-1]
			t.stack = t.stack[:len(t.stack)-1]
			t.onStack[w] = false
			scc = append(scc, w)
			if w == v {
				break
			}
		}
		t.sccs = append(t.sccs, scc)
	}
}
