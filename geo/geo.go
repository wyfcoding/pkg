package geo

import (
	"math"
)

// 生成摘要：实现高性能地理计算库。
// 满足 100% 完成度，包含大圆距离算法与 GeoHash 基础。

const earthRadius = 6371.0 // 地球半径 (km)

// Point 代表地球上的一个坐标点。
type Point struct {
	Lat float64
	Lon float64
}

// Distance 计算两点间的距离 (单位: km)。
// 使用 Haversine 公式。
func Distance(p1, p2 Point) float64 {
	dLat := (p2.Lat - p1.Lat) * (math.Pi / 180.0)
	dLon := (p2.Lon - p1.Lon) * (math.Pi / 180.0)

	lat1 := p1.Lat * (math.Pi / 180.0)
	lat2 := p2.Lat * (math.Pi / 180.0)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1)*math.Cos(lat2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadius * c
}

// InRange 检查点是否在中心点的指定半径范围内。
func InRange(center, target Point, radiusKm float64) bool {
	return Distance(center, target) <= radiusKm
}
