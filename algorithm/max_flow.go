package algorithm

import (
	"math"
	"sync"
)

// Edge 代表流网络中的一条边。
type Edge struct {
	To     int
	Cap    int64
	Flow   int64
	RevIdx int // 反向边在邻接表中的索引。
}

// DinicGraph 实现了基于分层图和 DFS 的 Dinic 最大流算法。
type DinicGraph struct {
	nodes int
	adj   [][]Edge
	level []int // 节点深度。
	ptr   []int // 当前弧优化：记录 DFS 遍历到哪个边了。
	queue []int // 复用 buffer。
	mu    sync.Mutex
}

// NewDinicGraph 创建一个新的 Dinic 图。
func NewDinicGraph(n int) *DinicGraph {
	return &DinicGraph{
		nodes: n,
		adj:   make([][]Edge, n),
		level: make([]int, n),
		ptr:   make([]int, n),
		queue: make([]int, 0, n),
	}
}

// AddEdge 添加有向边和对应的反向边。
func (g *DinicGraph) AddEdge(from, to int, capacity int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.adj[from] = append(g.adj[from], Edge{
		To:     to,
		Cap:    capacity,
		RevIdx: len(g.adj[to]),
	})
	g.adj[to] = append(g.adj[to], Edge{
		To:     from,
		Cap:    0,
		RevIdx: len(g.adj[from]) - 1,
	})
}

// bfs 构造分层图。
func (g *DinicGraph) bfs(s, t int) bool {
	for i := range g.level {
		g.level[i] = -1
	}
	g.level[s] = 0

	// 复用 queue。
	g.queue = g.queue[:0]
	g.queue = append(g.queue, s)

	for len(g.queue) > 0 {
		v := g.queue[0]
		g.queue = g.queue[1:]
		for _, edge := range g.adj[v] {
			if edge.Cap-edge.Flow > 0 && g.level[edge.To] == -1 {
				g.level[edge.To] = g.level[v] + 1
				g.queue = append(g.queue, edge.To)
			}
		}
	}
	return g.level[t] != -1
}

// dfs 寻找阻塞流。
func (g *DinicGraph) dfs(v, t int, pushed int64) int64 {
	if pushed == 0 || v == t {
		return pushed
	}

	for g.ptr[v] < len(g.adj[v]) {
		i := g.ptr[v]
		edge := &g.adj[v][i]
		if g.level[v]+1 != g.level[edge.To] || edge.Cap-edge.Flow == 0 {
			g.ptr[v]++
			continue
		}

		tr := g.dfs(edge.To, t, minInt64(pushed, edge.Cap-edge.Flow))
		if tr == 0 {
			g.ptr[v]++
			continue
		}

		edge.Flow += tr
		g.adj[edge.To][edge.RevIdx].Flow -= tr
		return tr
	}
	return 0
}

// MaxFlow 计算从 s 到 t 的最大流。
func (g *DinicGraph) MaxFlow(s, t int) int64 {
	var flow int64
	for g.bfs(s, t) {
		for i := range g.ptr {
			g.ptr[i] = 0
		}
		for {
			pushed := g.dfs(s, t, math.MaxInt64)
			if pushed == 0 {
				break
			}
			flow += pushed
		}
	}
	return flow
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
