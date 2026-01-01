package algorithm

import (
	"math"
	"sync"
)

// WeightedBipartiteGraph 结构体代表一个带权的二分图。
// 使用 KM (Kuhn-Munkres) 算法解决最大权/最小权完美匹配问题。
type WeightedBipartiteGraph struct {
	nx, ny     int         // 左右两侧节点数量
	weights    [][]float64 // 权重矩阵
	lx, ly     []float64   // 左右顶标
	matchX     []int       // 左侧匹配结果
	matchY     []int       // 右侧匹配结果
	slack      []float64   // 松弛量
	visX, visY []bool      // 访问标记
	mu         sync.Mutex
}

// NewWeightedBipartiteGraph 创建一个新的带权二分图
func NewWeightedBipartiteGraph(nx, ny int) *WeightedBipartiteGraph {
	maxNodes := nx
	if ny > nx {
		maxNodes = ny
	}

	weights := make([][]float64, maxNodes)
	for i := range weights {
		weights[i] = make([]float64, maxNodes)
		for j := range weights[i] {
			weights[i][j] = -math.MaxFloat64 // 默认极小值（用于最大权匹配）
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

// SetWeight 设置权重。如果是寻找最小权匹配，请传入 -weight
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
		if math.Abs(tmp) < 1e-9 {
			g.visY[v] = true
			if g.matchY[v] == -1 || g.dfs(g.matchY[v]) {
				g.matchY[v] = u
				g.matchX[u] = v
				return true
			}
		} else {
			if g.slack[v] > tmp {
				g.slack[v] = tmp
			}
		}
	}
	return false
}

// Solve 运行 KM 算法，返回最大权重和。
func (g *WeightedBipartiteGraph) Solve() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 1. 初始化
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

	// 2. 为每个左侧节点寻找匹配
	for i := 0; i < g.nx; i++ {
		for j := 0; j < g.ny; j++ {
			g.slack[j] = math.MaxFloat64
		}

		for {
			for j := 0; j < g.nx; j++ {
				g.visX[j] = false
				g.visY[j] = false
			}

			if g.dfs(i) {
				break
			}

			// 调整顶标
			d := math.MaxFloat64
			for j := 0; j < g.ny; j++ {
				if !g.visY[j] && g.slack[j] < d {
					d = g.slack[j]
				}
			}

			if d == math.MaxFloat64 {
				break // 无法匹配
			}

			for j := 0; j < g.nx; j++ {
				if g.visX[j] {
					g.lx[j] -= d
				}
			}
			for j := 0; j < g.ny; j++ {
				if g.visY[j] {
					g.ly[j] += d
				} else {
					g.slack[j] -= d
				}
			}
		}
	}

	res := 0.0
	for i := 0; i < g.ny; i++ {
		if g.matchY[i] != -1 {
			res += g.weights[g.matchY[i]][i]
		}
	}
	return res
}

// GetMatch 获取匹配结果 map[leftNodeIndex]rightNodeIndex
func (g *WeightedBipartiteGraph) GetMatch() map[int]int {
	res := make(map[int]int)
	for i := 0; i < g.nx; i++ {
		if g.matchX[i] != -1 {
			res[i] = g.matchX[i]
		}
	}
	return res
}
