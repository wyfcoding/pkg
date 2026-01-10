// Package algorithm 提供了高性能算法实现。
package algorithm

import (
	"math"
	"runtime"
	"slices"
	"sync"
)

// Location 代表地理位置。
type Location struct {
	ID     uint64
	Lat    float64
	Lon    float64
	Demand float64 // 该位置的载重需求量。
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
func (ro *RouteOptimizer) OptimizeRoute(start Location, destinations []Location) Route {
	destLen := len(destinations)
	if destLen == 0 {
		return Route{
			Locations: []Location{start},
			Distance:  0,
		}
	}

	visited := make(map[uint64]bool)
	route := make([]Location, 0, destLen+1)
	route = append(route, start)
	totalDistance := 0.0
	current := start

	for len(route) < destLen+1 {
		nearestIdx, nearestDist := ro.findNearest(current, destinations, visited)
		if nearestIdx == -1 {
			break
		}

		nearestLoc := destinations[nearestIdx]
		visited[nearestLoc.ID] = true
		route = append(route, nearestLoc)
		totalDistance += nearestDist
		current = nearestLoc
	}

	return Route{
		Locations: route,
		Distance:  totalDistance,
	}
}

func (ro *RouteOptimizer) findNearest(current Location, destinations []Location, visited map[uint64]bool) (int, float64) {
	minDist := math.MaxFloat64
	bestIdx := -1

	for idx, dest := range destinations {
		if visited[dest.ID] {
			continue
		}

		dist := HaversineDistance(current.Lat, current.Lon, dest.Lat, dest.Lon)
		if dist < minDist {
			minDist = dist
			bestIdx = idx
		}
	}

	return bestIdx, minDist
}

type saving struct {
	i, j int
	val  float64
}

// ClarkeWrightVRP 使用 Savings 算法优化带载重限制的多车路线 (CVRP)。
func (ro *RouteOptimizer) ClarkeWrightVRP(start Location, destinations []Location, capacity float64) []Route {
	destLen := len(destinations)
	if destLen == 0 {
		return nil
	}

	// 1. 初始化路线。
	routes := make([][]int, destLen)
	for idx := range destLen {
		routes[idx] = []int{idx}
	}

	// 2. 计算 Savings 并并行加速。
	savings := ro.parallelCalculateSavings(start, destinations)

	// 3. 按 Savings 降序排列。
	slices.SortFunc(savings, func(a, b saving) int {
		if a.val > b.val {
			return -1
		} else if a.val < b.val {
			return 1
		}

		return 0
	})

	// 4. 合并路线。
	finalRoutes := ro.processMerging(destinations, routes, savings, capacity)

	return ro.buildFinalRoutes(start, destinations, finalRoutes)
}

func (ro *RouteOptimizer) parallelCalculateSavings(start Location, destinations []Location) []saving {
	destLen := len(destinations)
	distToStart := make([]float64, destLen)

	for idx := range destLen {
		distToStart[idx] = HaversineDistance(start.Lat, start.Lon, destinations[idx].Lat, destinations[idx].Lon)
	}

	numWorkers := runtime.GOMAXPROCS(0)
	if destLen < 100 {
		numWorkers = 1
	}

	chunks := make([][]saving, numWorkers)
	var waitGrp sync.WaitGroup
	waitGrp.Add(numWorkers)

	for workerID := 0; workerID < numWorkers; workerID++ {
		go func(id int) {
			defer waitGrp.Done()
			var localSavings []saving
			for idxI := id; idxI < destLen; idxI += numWorkers {
				for idxJ := idxI + 1; idxJ < destLen; idxJ++ {
					distIJ := HaversineDistance(destinations[idxI].Lat, destinations[idxI].Lon, destinations[idxJ].Lat, destinations[idxJ].Lon)
					val := distToStart[idxI] + distToStart[idxJ] - distIJ
					if val > 0 {
						localSavings = append(localSavings, saving{i: idxI, j: idxJ, val: val})
					}
				}
			}
			chunks[id] = localSavings
		}(workerID)
	}
	waitGrp.Wait()

	var totalSavings []saving
	for _, chunk := range chunks {
		totalSavings = append(totalSavings, chunk...)
	}

	return totalSavings
}

func (ro *RouteOptimizer) processMerging(destinations []Location, currentRoutes [][]int, savings []saving, capacity float64) [][]int {
	routes := currentRoutes

	for _, sav := range savings {
		r1Idx, pos1 := ro.findRouteInfo(routes, sav.i)
		r2Idx, pos2 := ro.findRouteInfo(routes, sav.j)

		if r1Idx == -1 || r2Idx == -1 || r1Idx == r2Idx {
			continue
		}

		if pos1 == -1 || pos2 == -1 {
			continue
		}

		if ro.getRouteDemand(destinations, routes[r1Idx])+ro.getRouteDemand(destinations, routes[r2Idx]) <= capacity {
			newRoute := ro.mergeTwoRoutes(routes[r1Idx], routes[r2Idx], pos1, pos2)
			if newRoute != nil {
				routes[r1Idx] = newRoute
				routes = append(routes[:r2Idx], routes[r2Idx+1:]...)
			}
		}
	}

	return routes
}

func (ro *RouteOptimizer) findRouteInfo(routes [][]int, nodeIdx int) (int, int) {
	for rIdx, route := range routes {
		for pIdx, node := range route {
			if node == nodeIdx {
				pos := -1 // 默认在中间。
				if pIdx == 0 {
					pos = 0 // 头部。
				} else if pIdx == len(route)-1 {
					pos = 1 // 尾部。
				}

				return rIdx, pos
			}
		}
	}

	return -1, -1
}

func (ro *RouteOptimizer) getRouteDemand(destinations []Location, route []int) float64 {
	totalDemand := 0.0
	for _, idx := range route {
		totalDemand += destinations[idx].Demand
	}

	return totalDemand
}

func (ro *RouteOptimizer) mergeTwoRoutes(route1, route2 []int, pos1, pos2 int) []int {
	var merged []int

	if pos1 == 1 && pos2 == 0 { // 1尾连2头。
		merged = append(route1, route2...)
	} else if pos1 == 0 && pos2 == 1 { // 2尾连1头。
		merged = append(route2, route1...)
	} else if pos1 == 0 && pos2 == 0 { // 1头连2头（翻转1）。
		merged = make([]int, 0, len(route1)+len(route2))
		for idx := len(route1) - 1; idx >= 0; idx-- {
			merged = append(merged, route1[idx])
		}
		merged = append(merged, route2...)
	} else if pos1 == 1 && pos2 == 1 { // 1尾连2尾（翻转2）。
		merged = make([]int, 0, len(route1)+len(route2))
		merged = append(merged, route1...)
		for idx := len(route2) - 1; idx >= 0; idx-- {
			merged = append(merged, route2[idx])
		}
	}

	return merged
}

func (ro *RouteOptimizer) buildFinalRoutes(start Location, destinations []Location, routes [][]int) []Route {
	final := make([]Route, len(routes))

	for idx, routeNodes := range routes {
		locs := make([]Location, 0, len(routeNodes)+2)
		locs = append(locs, start)
		dist := 0.0
		curr := start

		for _, nodeIdx := range routeNodes {
			dest := destinations[nodeIdx]
			locs = append(locs, dest)
			dist += HaversineDistance(curr.Lat, curr.Lon, dest.Lat, dest.Lon)
			curr = dest
		}

		dist += HaversineDistance(curr.Lat, curr.Lon, start.Lat, start.Lon)
		final[idx] = Route{Locations: locs, Distance: dist}
	}

	return final
}