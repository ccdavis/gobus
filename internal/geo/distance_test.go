package geo

import (
	"math"
	"testing"
)

func TestHaversine_KnownDistances(t *testing.T) {
	tests := []struct {
		name                         string
		lat1, lon1, lat2, lon2       float64
		wantMeters                   float64
		tolerance                    float64 // allowed error in meters
	}{
		{
			name:       "Minneapolis to St Paul (~14 km)",
			lat1:       44.9778, lon1: -93.2650,
			lat2:       44.9537, lon2: -93.0900,
			wantMeters: 14_026,
			tolerance:  50,
		},
		{
			name:       "same point returns zero",
			lat1:       44.9778, lon1: -93.2650,
			lat2:       44.9778, lon2: -93.2650,
			wantMeters: 0,
			tolerance:  0.001,
		},
		{
			name:       "across a street (~100m)",
			lat1:       44.97780, lon1: -93.26500,
			lat2:       44.97780, lon2: -93.26370,
			wantMeters: 100,
			tolerance:  15,
		},
		{
			name:       "north pole to south pole",
			lat1:       90, lon1: 0,
			lat2:       -90, lon2: 0,
			wantMeters: math.Pi * earthRadiusMeters,
			tolerance:  1,
		},
		{
			name:       "equator quarter circumference",
			lat1:       0, lon1: 0,
			lat2:       0, lon2: 90,
			wantMeters: math.Pi / 2 * earthRadiusMeters,
			tolerance:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Haversine(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if math.Abs(got-tt.wantMeters) > tt.tolerance {
				t.Errorf("Haversine() = %.1f m, want %.1f m (±%.0f)", got, tt.wantMeters, tt.tolerance)
			}
		})
	}
}

func TestHaversine_Symmetry(t *testing.T) {
	a := Haversine(44.9778, -93.2650, 44.9537, -93.0900)
	b := Haversine(44.9537, -93.0900, 44.9778, -93.2650)
	if a != b {
		t.Errorf("Haversine not symmetric: %f != %f", a, b)
	}
}

func TestBoundingBoxRadius(t *testing.T) {
	// At the equator, 1 degree lat ≈ 111km and 1 degree lon ≈ 111km
	latDeg, lonDeg := BoundingBoxRadius(0, 111_000)
	if math.Abs(latDeg-1.0) > 0.01 {
		t.Errorf("latDeg at equator for 111km = %f, want ~1.0", latDeg)
	}
	if math.Abs(lonDeg-1.0) > 0.01 {
		t.Errorf("lonDeg at equator for 111km = %f, want ~1.0", lonDeg)
	}

	// At Minneapolis latitude (~45°), lonDeg should be larger than latDeg
	latDeg45, lonDeg45 := BoundingBoxRadius(45, 1000)
	if lonDeg45 <= latDeg45 {
		t.Errorf("at lat 45°, lonDeg (%f) should be > latDeg (%f)", lonDeg45, latDeg45)
	}
	// lonDeg should be roughly latDeg / cos(45°) ≈ latDeg * 1.414
	ratio := lonDeg45 / latDeg45
	if math.Abs(ratio-math.Sqrt(2)) > 0.01 {
		t.Errorf("lonDeg/latDeg ratio at 45° = %f, want ~1.414", ratio)
	}
}

func TestManhattanDistance(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		wantMeters             float64
		tolerance              float64
	}{
		{
			name:       "same point returns zero",
			lat1:       44.9778, lon1: -93.2650,
			lat2:       44.9778, lon2: -93.2650,
			wantMeters: 0,
			tolerance:  0.001,
		},
		{
			name:       "one block north (~200m) in Minneapolis",
			lat1:       44.97780, lon1: -93.26500,
			lat2:       44.97960, lon2: -93.26500,
			wantMeters: 200,
			tolerance:  5,
		},
		{
			name:       "diagonal should be greater than Haversine",
			lat1:       44.9778, lon1: -93.2650,
			lat2:       44.9800, lon2: -93.2630,
			wantMeters: 0, // just check > Haversine
			tolerance:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ManhattanDistance(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if tt.name == "diagonal should be greater than Haversine" {
				hav := Haversine(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
				if got <= hav {
					t.Errorf("ManhattanDistance() = %.1f should be > Haversine() = %.1f", got, hav)
				}
				return
			}
			if math.Abs(got-tt.wantMeters) > tt.tolerance {
				t.Errorf("ManhattanDistance() = %.1f m, want %.1f m (±%.0f)", got, tt.wantMeters, tt.tolerance)
			}
		})
	}
}

func TestManhattanDistance_Symmetry(t *testing.T) {
	a := ManhattanDistance(44.9778, -93.2650, 44.9537, -93.0900)
	b := ManhattanDistance(44.9537, -93.0900, 44.9778, -93.2650)
	if a != b {
		t.Errorf("ManhattanDistance not symmetric: %f != %f", a, b)
	}
}

func TestMetersToMiles(t *testing.T) {
	tests := []struct {
		meters float64
		want   float64
	}{
		{0, 0},
		{1609.344, 1.0},
		{3218.688, 2.0},
	}
	for _, tt := range tests {
		got := MetersToMiles(tt.meters)
		if math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("MetersToMiles(%f) = %f, want %f", tt.meters, got, tt.want)
		}
	}
}
