package handler

import (
	"strings"
	"testing"
)

func TestFormatDistance(t *testing.T) {
	tests := []struct {
		meters float64
		want   string
	}{
		{50, "50 m"},
		{100, "100 m"},
		{500, "500 m"},
		{999, "999 m"},
		{1000, "0.6 mi"},
		{1609, "1.0 mi"},
		{3218, "2.0 mi"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDistance(tt.meters)
			if got != tt.want {
				t.Errorf("formatDistance(%f) = %q, want %q", tt.meters, got, tt.want)
			}
		})
	}
}

func TestFormatStopDesc(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Nearside S", "Southbound side"},
		{"Farside N", "Northbound side"},
		{"Nearside E", "Eastbound side"},
		{"Farside W", "Westbound side"},
		{"", ""},
		{"  ", ""},
		{"Unknown format", "Unknown format"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := formatStopDesc(tt.input)
			if got != tt.want {
				t.Errorf("formatStopDesc(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNextRadius(t *testing.T) {
	tests := []struct {
		name       string
		current    float64
		wantRadius float64
		wantOK     bool
	}{
		{"zero gets first tier", 0, 400, true},
		{"below first tier", 200, 400, true},
		{"at first tier", 400, 800, true},
		{"between tiers", 600, 800, true},
		{"at second tier", 800, 1600, true},
		{"at third tier", 1600, 3200, true},
		{"at fourth tier", 3200, 6400, true},
		{"at fifth tier", 6400, 12800, true},
		{"at max tier", 12800, 0, false},
		{"above max tier", 20000, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRadius, gotOK := nextRadius(tt.current)
			if gotRadius != tt.wantRadius || gotOK != tt.wantOK {
				t.Errorf("nextRadius(%v) = (%v, %v), want (%v, %v)",
					tt.current, gotRadius, gotOK, tt.wantRadius, tt.wantOK)
			}
		})
	}
}

func TestRadiusTiersOrdering(t *testing.T) {
	for i := 1; i < len(radiusTiers); i++ {
		if radiusTiers[i] <= radiusTiers[i-1] {
			t.Errorf("radiusTiers not strictly increasing: [%d]=%v >= [%d]=%v",
				i-1, radiusTiers[i-1], i, radiusTiers[i])
		}
	}
}

func TestDbLimitForRadius(t *testing.T) {
	tests := []struct {
		name        string
		radius      float64
		wantDB      int
		wantDisplay int
	}{
		{"very small radius", 200, 10, 5},
		{"at 400m", 400, 10, 5},
		{"at 600m", 600, 20, 10},
		{"at 800m", 800, 20, 10},
		{"at 1200m", 1200, 30, 15},
		{"at 1600m", 1600, 30, 15},
		{"at 3200m", 3200, 50, 20},
		{"at 5000m", 5000, 75, 30},
		{"at 6400m", 6400, 75, 30},
		{"at 12800m", 12800, 100, 40},
		{"very large", 50000, 100, 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDB, gotDisplay := dbLimitForRadius(tt.radius)
			if gotDB != tt.wantDB || gotDisplay != tt.wantDisplay {
				t.Errorf("dbLimitForRadius(%v) = (%v, %v), want (%v, %v)",
					tt.radius, gotDB, gotDisplay, tt.wantDB, tt.wantDisplay)
			}
		})
	}
}

func TestBuildRoutesMoreURL(t *testing.T) {
	got := buildRoutesMoreURL("44.97", "-93.27", 5, 1600)
	want := "/nearby?view=routes&lat=44.97&lon=-93.27&offset=5&radius=1600&partial=1"
	if got != want {
		t.Errorf("buildRoutesMoreURL() = %q, want %q", got, want)
	}
}

func TestBuildStopsMoreURL(t *testing.T) {
	got := buildStopsMoreURL("44.97", "-93.27", 10, 3200)
	want := "/nearby?view=stops&lat=44.97&lon=-93.27&offset=10&radius=3200&partial=1"
	if got != want {
		t.Errorf("buildStopsMoreURL() = %q, want %q", got, want)
	}
}

func TestBuildMoreURL_ContainsAllParams(t *testing.T) {
	// Verify routes URL contains all required parameters
	routeURL := buildRoutesMoreURL("44.97", "-93.27", 15, 6400)
	for _, param := range []string{"view=routes", "lat=44.97", "lon=-93.27", "offset=15", "radius=6400", "partial=1"} {
		if !strings.Contains(routeURL, param) {
			t.Errorf("buildRoutesMoreURL() missing param %q in %q", param, routeURL)
		}
	}

	// Verify stops URL contains all required parameters
	stopURL := buildStopsMoreURL("44.97", "-93.27", 20, 12800)
	for _, param := range []string{"view=stops", "lat=44.97", "lon=-93.27", "offset=20", "radius=12800", "partial=1"} {
		if !strings.Contains(stopURL, param) {
			t.Errorf("buildStopsMoreURL() missing param %q in %q", param, stopURL)
		}
	}
}
