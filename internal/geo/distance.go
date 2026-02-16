package geo

import "math"

const earthRadiusMeters = 6_371_000

// Haversine returns the great-circle distance in meters between two lat/lon points.
func Haversine(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusMeters * c
}

// BoundingBoxRadius returns the approximate degree offset for a given radius in meters
// at the specified latitude. Returns (latDeg, lonDeg).
func BoundingBoxRadius(lat, radiusMeters float64) (latDeg, lonDeg float64) {
	latDeg = radiusMeters / earthRadiusMeters * (180 / math.Pi)
	lonDeg = latDeg / math.Cos(toRad(lat))
	return latDeg, lonDeg
}

// MetersToMiles converts meters to miles.
func MetersToMiles(m float64) float64 {
	return m / 1609.344
}

func toRad(deg float64) float64 {
	return deg * math.Pi / 180
}
