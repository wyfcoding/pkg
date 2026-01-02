package algorithm

import (
	"math"
	"sort"
)

// Location 代表地理位置。
type Location struct {
	ID     uint64
	Lat    float64
	Lon    float64
	Demand float64 // 该位置的载重需求量
}

// Route 代表优化后的路线。
type Route struct {
	Locations []Location
	Distance  float64
}

// RouteOptimizer 优化配送路线。
type RouteOptimizer struct{}

// NewRouteOptimizer 创建一个新的 RouteOptimizer。
func NewRouteOptimizer() *RouteOptimizer {
	return &RouteOptimizer{}
}

// OptimizeRoute 优化从起点开始访问所有地点的路线。
// 使用简单的最近邻算法解决 TSP 问题。
func (ro *RouteOptimizer) OptimizeRoute(start Location, destinations []Location) Route {
	if len(destinations) == 0 {
		return Route{
			Locations: []Location{start},
			Distance:  0,
		}
	}

	visited := make(map[uint64]bool)
	route := []Location{start}
	totalDistance := 0.0
	current := start

	for len(route) < len(destinations)+1 {
		nearestDist := math.MaxFloat64
		var nearestLoc Location
		found := false

		for _, dest := range destinations {
			if visited[dest.ID] {
				continue
			}

			dist := HaversineDistance(current.Lat, current.Lon, dest.Lat, dest.Lon)
			if dist < nearestDist {
				nearestDist = dist
				nearestLoc = dest
				found = true
			}
		}

		if found {
			visited[nearestLoc.ID] = true
			route = append(route, nearestLoc)
			totalDistance += nearestDist
			current = nearestLoc
		} else {
			break
		}
	}

	return Route{
		Locations: route,
		Distance:  totalDistance,
	}
}

// ClarkeWrightVRP 使用 Savings 算法优化带载重限制的多车路线 (CVRP)。
func (ro *RouteOptimizer) ClarkeWrightVRP(start Location, destinations []Location, capacity float64) []Route {
	if len(destinations) == 0 {
		return nil
	}

	// 1. 初始化：每辆车只服务一个目的地 (start -> dest -> start)
	type saving struct {
		i, j int
		val  float64
	}
	n := len(destinations)
	routes := make([][]int, n)
	for i := 0; i < n; i++ {
		routes[i] = []int{i}
	}

	// 2. 计算 Savings：S(i,j) = d(0,i) + d(0,j) - d(i,j)
	savings := make([]saving, 0, n*(n-1)/2)
	distToStart := make([]float64, n)
	for i := 0; i < n; i++ {
		distToStart[i] = HaversineDistance(start.Lat, start.Lon, destinations[i].Lat, destinations[i].Lon)
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			d_ij := HaversineDistance(destinations[i].Lat, destinations[i].Lon, destinations[j].Lat, destinations[j].Lon)
			s := distToStart[i] + distToStart[j] - d_ij
			if s > 0 {
				savings = append(savings, saving{i, j, s})
			}
		}
	}

	// 按节省值降序排列
	sort.Slice(savings, func(i, j int) bool {
		return savings[i].val > savings[j].val
	})

	// 3. 合并路径逻辑
	findRouteInfo := func(nodeIdx int) (int, int) { 
		for rIdx, r := range routes {
			for pIdx, node := range r {
				if node == nodeIdx {
					pos := -1 // 默认在中间
					if pIdx == 0 {
						pos = 0 // 头部
					} else if pIdx == len(r)-1 {
						pos = 1 // 尾部
					}
					return rIdx, pos
				}
			}
		}
		return -1, -1
	}

	getRouteDemand := func(r []int) float64 {
		sum := 0.0
		for _, idx := range r {
			sum += destinations[idx].Demand
		}
		return sum
	}

	for _, s := range savings {
		r1Idx, pos1 := findRouteInfo(s.i)
		r2Idx, pos2 := findRouteInfo(s.j)

		if r1Idx == -1 || r2Idx == -1 || r1Idx == r2Idx {
			continue
		}

		// 只有当两个节点分别位于各自路径的端点时才能合并
		if pos1 != -1 && pos2 != -1 {
			if getRouteDemand(routes[r1Idx])+getRouteDemand(routes[r2Idx]) <= capacity {
				// 执行合并逻辑
				var newRoute []int
				if pos1 == 1 && pos2 == 0 { // 路径1尾部连路径2头部
					newRoute = append(routes[r1Idx], routes[r2Idx]...)
				} else if pos1 == 0 && pos2 == 1 { // 路径2尾部连路径1头部
					newRoute = append(routes[r2Idx], routes[r1Idx]...)
				} else if pos1 == 0 && pos2 == 0 { // 翻转路径1，头部连头部
					for i := len(routes[r1Idx]) - 1; i >= 0; i-- {
						newRoute = append(newRoute, routes[r1Idx][i])
					}
					newRoute = append(newRoute, routes[r2Idx]...)
				} else if pos1 == 1 && pos2 == 1 { // 路径1尾部连路径2尾部（翻转路径2）
					newRoute = append([]int{}, routes[r1Idx]...)
					for i := len(routes[r2Idx]) - 1; i >= 0; i-- {
						newRoute = append(newRoute, routes[r2Idx][i])
					}
				}

				if newRoute != nil {
					routes[r1Idx] = newRoute
					// 移除已合并的路径2
					routes = append(routes[:r2Idx], routes[r2Idx+1:]...)
				}
			}
		}
	}

	// 4. 构建最终 Route 对象
	finalRoutes := make([]Route, len(routes))
	for i, r := range routes {
		locs := []Location{start}
		dist := 0.0
		curr := start
		for _, nodeIdx := range r {
			dest := destinations[nodeIdx]
			locs = append(locs, dest)
			dist += HaversineDistance(curr.Lat, curr.Lon, dest.Lat, dest.Lon)
			curr = dest
		}
		// 回到原点
		dist += HaversineDistance(curr.Lat, curr.Lon, start.Lat, start.Lon)
		finalRoutes[i] = Route{Locations: locs, Distance: dist}
	}

	return finalRoutes
}