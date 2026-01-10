package algorithm

import (
	"math"
)

// Shared constants for FNV-1a hash algorithm.
const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// Earth related constants for geographical distance calculations.
const (
	earthRadiusMeters = 6371000.0
	degreeToRadFactor = math.Pi / 180.0
)

// HaversineDistance calculates the great-circle distance between two points on a sphere.
func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	lat1Rad := lat1 * degreeToRadFactor
	lat2Rad := lat2 * degreeToRadFactor
	dLat := (lat2 - lat1) * degreeToRadFactor
	dLon := (lon2 - lon1) * degreeToRadFactor

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusMeters * c
}
