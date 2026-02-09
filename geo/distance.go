// Package geo 提供了地理位置计算工具。
// 增强：实现了 Haversine 算法用于计算两点间的球面距离，适用于物流与分拨中心计算。
package geo

import (
	"math"
)

const earthRadiusKm = 6371.0

// Point 表示一个地理经纬度坐标点。
type Point struct {
	Lat float64 // 纬度
	Lon float64 // 经度
}

// Distance 计算两点间的距离（单位：千米）。
// 使用 Haversine 公式。
func Distance(p1, p2 Point) float64 {
	lat1 := p1.Lat * math.Pi / 180
	lon1 := p1.Lon * math.Pi / 180
	lat2 := p2.Lat * math.Pi / 180
	lon2 := p2.Lon * math.Pi / 180

	dLat := lat2 - lat1
	dLon := lon2 - lon1

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

// WithinRange 检查两点是否在指定范围内（单位：千米）。
func WithinRange(p1, p2 Point, rangeKm float64) bool {
	return Distance(p1, p2) <= rangeKm
}
