// Package algorithm 提供了高性能算法实现.
package algorithm

import (
	"math"
	"sync"
)

// WeightedBipartiteGraph 实现了 KM 算法 (Kuhn-Munkres) 用于寻找二分图的最大权完美匹配.
// 优化后的实现采用 BFS + Slack 数组，复杂度为 O(N^3).
type WeightedBipartiteGraph struct {
	weights [][]int
	lx      []int
	ly      []int
	matchY  []int
	slack   []int
	pre     []int
	visY    []bool
	mu      sync.RWMutex
	nx      int
	ny      int
}

// NewWeightedBipartiteGraph 初始化二分图.
func NewWeightedBipartiteGraph(nx, ny int) *WeightedBipartiteGraph {
	weights := make([][]int, nx)
	for i := range nx {
		weights[i] = make([]int, ny)
	}

	return &WeightedBipartiteGraph{
		weights: weights,
		lx:      make([]int, nx),
		ly:      make([]int, ny),
		matchY:  make([]int, ny),
		slack:   make([]int, ny),
		pre:     make([]int, ny),
		visY:    make([]bool, ny),
		nx:      nx,
		ny:      ny,
	}
}

// AddEdge 添加边.
func (g *WeightedBipartiteGraph) AddEdge(u, v, weight int) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if u >= 0 && u < g.nx && v >= 0 && v < g.ny {
		g.weights[u][v] = weight
	}
}

// bfs 寻找增广路.
func (g *WeightedBipartiteGraph) bfs(u int) {
	for i := range g.ny {
		g.slack[i] = math.MaxInt
		g.visY[i] = false
		g.pre[i] = -1
	}

	var x, y, delta int
	x = u
	y = -1
	for {
		if y != -1 {
			g.visY[y] = true
			x = g.matchY[y]
		}

		delta = math.MaxInt
		nextY := -1
		for j := range g.ny {
			if !g.visY[j] {
				if g.lx[x]+g.ly[j]-g.weights[x][j] < g.slack[j] {
					g.slack[j] = g.lx[x] + g.ly[j] - g.weights[x][j]
					g.pre[j] = y
				}
				if g.slack[j] < delta {
					delta = g.slack[j]
					nextY = j
				}
			}
		}

		if delta > 0 {
			g.lx[u] -= delta
			for i := range g.ny {
				if g.visY[i] {
					g.lx[g.matchY[i]] -= delta
					g.ly[i] += delta
				} else {
					g.slack[i] -= delta
				}
			}
		}

		y = nextY
		if g.matchY[y] == -1 {
			break
		}
	}

	for y != -1 {
		prevY := g.pre[y]
		if prevY != -1 {
			g.matchY[y] = g.matchY[prevY]
		} else {
			g.matchY[y] = u
		}
		y = prevY
	}
}

// MaxWeightMatch 执行 KM 算法并返回最大权和.
func (g *WeightedBipartiteGraph) MaxWeightMatch() int {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 初始化顶标
	for i := range g.nx {
		g.lx[i] = math.MinInt
		for j := range g.ny {
			if g.weights[i][j] > g.lx[i] {
				g.lx[i] = g.weights[i][j]
			}
		}
	}

	for i := range g.ny {
		g.ly[i] = 0
		g.matchY[i] = -1
	}

	for i := range g.nx {
		g.bfs(i)
	}

	res := 0
	for i := range g.ny {
		if g.matchY[i] != -1 {
			res += g.weights[g.matchY[i]][i]
		}
	}

	return res
}

// GetMatches 返回最终匹配结果.
func (g *WeightedBipartiteGraph) GetMatches() map[int]int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	matches := make(map[int]int)
	for i := range g.ny {
		if g.matchY[i] != -1 {
			matches[g.matchY[i]] = i
		}
	}

	return matches
}
