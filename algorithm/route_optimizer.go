// Package algorithm 提供了高性能算法实现.
package algorithm

import (
	"math"
	"runtime"
	"slices"
	"sync"
)

const (
	minDestLenForParallel = 100
)

// Location 代表地理位置.
type Location struct {
	Lat    float64
	Lon    float64
	Demand float64
	ID     uint64
}

// Route 代表优化后的路线.
type Route struct {
	Locations []Location
	Distance  float64
}

// RouteOptimizer 优化配送路线.
type RouteOptimizer struct{}

// NewRouteOptimizer 创建一个新的 RouteOptimizer.
func NewRouteOptimizer() *RouteOptimizer {
	return &RouteOptimizer{}
}

// OptimizeRoute 优化从起点开始访问所有地点的路线.
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
		idx, dist := ro.findNearest(current, destinations, visited)
		if idx == -1 {
			break
		}

		nearest := destinations[idx]
		visited[nearest.ID] = true
		route = append(route, nearest)
		totalDistance += dist
		current = nearest
	}

	return Route{
		Locations: route,
		Distance:  totalDistance,
	}
}

func (ro *RouteOptimizer) findNearest(curr Location, dests []Location, visited map[uint64]bool) (bestIdx int, minDist float64) {
	minDist = math.MaxFloat64
	bestIdx = -1

	for idx, dest := range dests {
		if visited[dest.ID] {
			continue
		}

		dist := HaversineDistance(curr.Lat, curr.Lon, dest.Lat, dest.Lon)
		if dist < minDist {
			minDist = dist
			bestIdx = idx
		}
	}

	return bestIdx, minDist
}

type saving struct { //nolint:govet
	val float64
	i   int
	j   int
}

// ClarkeWrightVRP 使用 Savings 算法优化带载重限制的多车路线 (CVRP).
func (ro *RouteOptimizer) ClarkeWrightVRP(start Location, destinations []Location, capacity float64) []Route {
	destLen := len(destinations)
	if destLen == 0 {
		return nil
	}

	routes := make([][]int, destLen)
	for i := range destLen {
		routes[i] = []int{i}
	}

	savings := ro.parallelCalculateSavings(start, destinations)

	slices.SortFunc(savings, func(a, b saving) int {
		if a.val > b.val {
			return -1
		}

		if a.val < b.val {
			return 1
		}

		return 0
	})

	final := ro.processMerging(destinations, routes, savings, capacity)

	return ro.buildFinalRoutes(start, destinations, final)
}

func (ro *RouteOptimizer) parallelCalculateSavings(start Location, dests []Location) []saving {
	n := len(dests)
	distToStart := make([]float64, n)

	for i := range n {
		distToStart[i] = HaversineDistance(start.Lat, start.Lon, dests[i].Lat, dests[i].Lon)
	}

	numWorkers := runtime.GOMAXPROCS(0)
	if n < minDestLenForParallel {
		numWorkers = 1
	}

	chunks := make([][]saving, numWorkers)
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := range numWorkers {
		go func(workerID int) {
			defer wg.Done()
			var local []saving
			for i := workerID; i < n; i += numWorkers {
				for j := i + 1; j < n; j++ {
					distIJ := HaversineDistance(dests[i].Lat, dests[i].Lon, dests[j].Lat, dests[j].Lon)
					val := distToStart[i] + distToStart[j] - distIJ
					if val > 0 {
						local = append(local, saving{i: i, j: j, val: val})
					}
				}
			}
			chunks[workerID] = local
		}(w)
	}
	wg.Wait()

	var total []saving
	for _, chunk := range chunks {
		total = append(total, chunk...)
	}

	return total
}

func (ro *RouteOptimizer) processMerging(dests []Location, curr [][]int, savings []saving, maxCapacity float64) [][]int {
	res := curr

	for _, s := range savings {
		r1, p1 := ro.findRouteInfo(res, s.i)
		r2, p2 := ro.findRouteInfo(res, s.j)

		if r1 == -1 || r2 == -1 || r1 == r2 {
			continue
		}

		if p1 == -1 || p2 == -1 {
			continue
		}

		if ro.getRouteDemand(dests, res[r1])+ro.getRouteDemand(dests, res[r2]) <= maxCapacity {
			merged := ro.mergeTwoRoutes(res[r1], res[r2], p1, p2)
			if merged != nil {
				res[r1] = merged
				res = append(res[:r2], res[r2+1:]...)
			}
		}
	}

	return res
}

func (ro *RouteOptimizer) findRouteInfo(routes [][]int, nodeIdx int) (rIndex, pos int) {
	for rIdx, route := range routes {
		for pIdx, node := range route {
			if node == nodeIdx {
				pos = -1
				if pIdx == 0 {
					pos = 0
				} else if pIdx == len(route)-1 {
					pos = 1
				}

				return rIdx, pos
			}
		}
	}

	return -1, -1
}

func (ro *RouteOptimizer) getRouteDemand(dests []Location, route []int) float64 {
	var total float64
	for _, idx := range route {
		total += dests[idx].Demand
	}

	return total
}

func (ro *RouteOptimizer) mergeTwoRoutes(r1, r2 []int, p1, p2 int) []int {
	var res []int

	switch {
	case p1 == 1 && p2 == 0:
		r1 = append(r1, r2...)
		res = r1
	case p1 == 0 && p2 == 1:
		r2 = append(r2, r1...)
		res = r2
	case p1 == 0 && p2 == 0:
		res = make([]int, 0, len(r1)+len(r2))
		for i := range r1[:len(r1)-1] {
			res = append(res, r1[i])
		}
		res = append(res, r2...)
	case p1 == 1 && p2 == 1:
		res = make([]int, 0, len(r1)+len(r2))
		res = append(res, r1...)
		for i := len(r2) - 1; i >= 0; i-- {
			res = append(res, r2[i])
		}
	}

	return res
}

func (ro *RouteOptimizer) buildFinalRoutes(start Location, dests []Location, routes [][]int) []Route {
	res := make([]Route, len(routes))

	for i, nodes := range routes {
		locs := make([]Location, 0, len(nodes)+2)
		locs = append(locs, start)
		var totalDist float64
		curr := start

		for _, nIdx := range nodes {
			d := dests[nIdx]
			locs = append(locs, d)
			totalDist += HaversineDistance(curr.Lat, curr.Lon, d.Lat, d.Lon)
			curr = d
		}

		totalDist += HaversineDistance(curr.Lat, curr.Lon, start.Lat, start.Lon)
		res[i] = Route{Locations: locs, Distance: totalDist}
	}

	return res
}
