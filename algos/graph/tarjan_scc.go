// Package algorithm 提供高性能算法集合。
// 此文件实现了 Tarjan 强连通分量 (Strongly Connected Components, SCC) 算法。
//
// 算法原理.
// 基于深度优先搜索 (DFS)，利用栈和两个辅助数组（indices 和 lowlink）在单次遍历中识别有向图中的所有强连通分量。
//
// 复杂度分析.
// - 时间复杂度: O(V + E)，其中 V 为顶点数，E 为边数。
// - 空间复杂度: O(V)，用于存储递归栈和节点状态。
package graph

// Graph 定义了一个基于邻接表的有向.
type Graph struct {
	adj   [][]int // 邻接表
	nodes int     // 顶点总数
}

// NewGraph 创建一个新的有向图实.
func NewGraph(nodes int) *Graph {
	return &Graph{
		nodes: nodes,
		adj:   make([][]int, nodes),
	}
}

// AddEdge 向图中添加一条从 u 到 v 的有向.
func (g *Graph) AddEdge(u, v int) {
	g.adj[u] = append(g.adj[u], v)
}

// TarjanSCC 封装了计算强连通分量的状态。
type TarjanSCC struct {
	graph   *Graph
	stack   []int   // 用于存储当前遍历路径的栈。
	indices []int   // 节点的发现次序。
	lowlink []int   // 节点通过回边能到达的最小次序。
	sccs    [][]int // 最终识别出的强连通分量列表。
	onStack []bool  // 快速判断节点是否在栈中。
	index   int     // 当前 DFS 遍历的次序计数。
}

// NewTarjanSCC 初始化 Tarjan 算法执行器。
func NewTarjanSCC(g *Graph) *TarjanSCC {
	return &TarjanSCC{
		graph:   g,
		indices: make([]int, g.nodes),
		lowlink: make([]int, g.nodes),
		onStack: make([]bool, g.nodes),
		index:   -1,
	}
}

// Run 执行算法并返回所有的强连通分量。
func (t *TarjanSCC) Run() [][]int {
	for i := range t.indices {
		t.indices[i] = -1
	}

	for i := range t.graph.nodes {
		if t.indices[i] == -1 {
			t.strongConnect(i)
		}
	}
	return t.sccs
}

// strongConnect 是 Tarjan 算法的核心递归 DFS 函数。
func (t *TarjanSCC) strongConnect(v int) {
	t.index++
	t.indices[v] = t.index
	t.lowlink[v] = t.index
	t.stack = append(t.stack, v)
	t.onStack[v] = true

	// 遍历当前节点的所有邻居。
	for _, w := range t.graph.adj[v] {
		if t.indices[w] == -1 {
			// 邻居未被访问，递归访问。
			t.strongConnect(w)
			t.lowlink[v] = min(t.lowlink[v], t.lowlink[w])
		} else if t.onStack[w] {
			// 邻居在栈中，说明构成环。
			t.lowlink[v] = min(t.lowlink[v], t.indices[w])
		}
	}

	// 如果 lowlink 等于发现次序，说明 v 是一个强连通分量的根节点。
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
