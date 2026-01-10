package math

import (
	"math"
)

// Shared constants for FNV-1a hash algorithm.
const (
	FnvOffset64 = 14695981039346656037
	FnvPrime64  = 1099511628211
)

// Earth related constants for geographical distance calculations.
const (
	EarthRadiusMeters = 6371000.0
	DegreeToRadFactor = math.Pi / 180.0
)

// HaversineDistance calculates the great-circle distance between two points on a sphere.
func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	lat1Rad := lat1 * DegreeToRadFactor
	lat2Rad := lat2 * DegreeToRadFactor
	dLat := (lat2 - lat1) * DegreeToRadFactor
	dLon := (lon2 - lon1) * DegreeToRadFactor

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return EarthRadiusMeters * c
}
