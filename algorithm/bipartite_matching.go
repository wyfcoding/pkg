// Package algorithm 提供了高性能算法实现。
package algorithm

import (
	"log/slog"
	"math"
	"sync"
	"time"
)

// WeightedBipartiteGraph 结构体代表一个带权的二分图。
// 使用 KM (Kuhn-Munkres) 算法解决最大权完美匹配问题。
type WeightedBipartiteGraph struct {
	nx, ny     int         // 左右两侧节点数。
	weights    [][]float64 // 权重矩阵。
	lx, ly     []float64   // 左右顶标。
	matchX     []int       // 左侧匹配结果。
	matchY     []int       // 右侧匹配结果。
	slack      []float64   // 松弛量。
	visX, visY []bool      // 访问标志。
	mu         sync.Mutex
}

const eps = 1e-9

// NewWeightedBipartiteGraph 创建一个新的带权二分图。
func NewWeightedBipartiteGraph(nx, ny int) *WeightedBipartiteGraph {
	maxNodes := nx
	if ny > maxNodes {
		maxNodes = ny
	}

	weights := make([][]float64, maxNodes)
	for i := range weights {
		weights[i] = make([]float64, maxNodes)
		for j := range weights[i] {
			weights[i][j] = -math.MaxFloat64 // 默认极小值。
		}
	}

	return &WeightedBipartiteGraph{
		nx:      nx,
		ny:      ny,
		weights: weights,
		lx:      make([]float64, maxNodes),
		ly:      make([]float64, maxNodes),
		matchX:  make([]int, maxNodes),
		matchY:  make([]int, maxNodes),
		slack:   make([]float64, maxNodes),
		visX:    make([]bool, maxNodes),
		visY:    make([]bool, maxNodes),
	}
}

// SetWeight 设置权重。
func (g *WeightedBipartiteGraph) SetWeight(u, v int, weight float64) {
	g.weights[u][v] = weight
}

func (g *WeightedBipartiteGraph) dfs(u int) bool {
	g.visX[u] = true

	for v := 0; v < g.ny; v++ {
		if g.visY[v] {
			continue
		}

		tmp := g.lx[u] + g.ly[v] - g.weights[u][v]
		if math.Abs(tmp) < eps {
			g.visY[v] = true
			if g.matchY[v] == -1 || g.dfs(g.matchY[v]) {
				g.matchY[v] = u
				g.matchX[u] = v

				return true
			}
		} else if g.slack[v] > tmp {
			g.slack[v] = tmp
		}
	}

	return false
}

// Solve 运行 KM 算法，返回最大权重和。
func (g *WeightedBipartiteGraph) Solve() float64 {
	startTime := time.Now()

	g.mu.Lock()
	defer g.mu.Unlock()

	g.initializeLabels()

	for i := 0; i < g.nx; i++ {
		for j := 0; j < g.ny; j++ {
			g.slack[j] = math.MaxFloat64
		}

		for {
			g.resetVisits()

			if g.dfs(i) {
				break
			}

			if !g.adjustLabels() {
				break
			}
		}
	}

	return g.calculateResult(startTime)
}

func (g *WeightedBipartiteGraph) initializeLabels() {
	for i := 0; i < g.nx; i++ {
		g.matchX[i] = -1
		g.matchY[i] = -1
		g.lx[i] = -math.MaxFloat64
		g.ly[i] = 0

		for j := 0; j < g.ny; j++ {
			if g.weights[i][j] > g.lx[i] {
				g.lx[i] = g.weights[i][j]
			}
		}
	}
}

func (g *WeightedBipartiteGraph) resetVisits() {
	for j := 0; j < g.nx; j++ {
		g.visX[j] = false
	}

	for j := 0; j < g.ny; j++ {
		g.visY[j] = false
	}
}

func (g *WeightedBipartiteGraph) adjustLabels() bool {
	delta := math.MaxFloat64
	for j := 0; j < g.ny; j++ {
		if !g.visY[j] && g.slack[j] < delta {
			delta = g.slack[j]
		}
	}

	if delta == math.MaxFloat64 {
		return false
	}

	for j := 0; j < g.nx; j++ {
		if g.visX[j] {
			g.lx[j] -= delta
		}
	}

	for j := 0; j < g.ny; j++ {
		if g.visY[j] {
			g.ly[j] += delta
		} else {
			g.slack[j] -= delta
		}
	}

	return true
}

func (g *WeightedBipartiteGraph) calculateResult(startTime time.Time) float64 {
	totalWeight := 0.0
	matchCount := 0

	for i := 0; i < g.ny; i++ {
		if g.matchY[i] != -1 {
			totalWeight += g.weights[g.matchY[i]][i]
			matchCount++
		}
	}

	slog.Info("Weighted bipartite matching Solve completed",
		"nx", g.nx,
		"ny", g.ny,
		"matches", matchCount,
		"weight_sum", totalWeight,
		"duration", time.Since(startTime))

	return totalWeight
}

// GetMatch 获取匹配结果 map[leftNodeIndex]rightNodeIndex。
func (g *WeightedBipartiteGraph) GetMatch() map[int]int {
	res := make(map[int]int)

	for i := 0; i < g.nx; i++ {
		if g.matchX[i] != -1 {
			res[i] = g.matchX[i]
		}
	}

	return res
}