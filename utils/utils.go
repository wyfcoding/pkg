package utils

import (
	"crypto/rand"
	"encoding/hex"
	"math"
)

// GenerateRandomID 生成指定长度的随机十六进制字符串 ID.
func GenerateRandomID(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

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
