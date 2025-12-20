package algorithm

import (
	"math"
	"sort"
)

// Location 代表地理位置。
type Location struct {
	ID  uint64
	Lat float64
	Lon float64
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

			dist := haversineDistance(current.Lat, current.Lon, dest.Lat, dest.Lon)
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

// OptimizeBatchRoutes 优化多辆车的路线 (简化的 VRP)。
// 使用 K-Means (简化版) 对地点进行聚类，然后对每个聚类进行 TSP。
func (ro *RouteOptimizer) OptimizeBatchRoutes(start Location, destinations []Location, vehicles int) []Route {
	if len(destinations) == 0 {
		return []Route{}
	}
	if vehicles <= 0 {
		vehicles = 1
	}
	if len(destinations) <= vehicles {
		// 简单情况：每辆车分配一个或少数几个目的地
		routes := make([]Route, 0)
		for _, dest := range destinations {
			dist := haversineDistance(start.Lat, start.Lon, dest.Lat, start.Lon)
			routes = append(routes, Route{
				Locations: []Location{start, dest},
				Distance:  dist,
			})
		}
		return routes
	}

	// 1. 基于与起点的角度对目的地进行聚类 (简化的扫描算法)
	// 计算每个目的地的角度
	type destWithAngle struct {
		Location
		angle float64
	}
	dests := make([]destWithAngle, len(destinations))
	for i, d := range destinations {
		angle := math.Atan2(d.Lat-start.Lat, d.Lon-start.Lon)
		dests[i] = destWithAngle{d, angle}
	}

	// 按角度排序
	sort.Slice(dests, func(i, j int) bool {
		return dests[i].angle < dests[j].angle
	})

	// 分块
	chunkSize := (len(dests) + vehicles - 1) / vehicles
	routes := make([]Route, 0)

	for i := 0; i < len(dests); i += chunkSize {
		end := i + chunkSize
		if end > len(dests) {
			end = len(dests)
		}

		chunk := make([]Location, end-i)
		for j := range chunk {
			chunk[j] = dests[i+j].Location
		}

		// 优化该分块的路径
		routes = append(routes, ro.OptimizeRoute(start, chunk))
	}

	return routes
}
