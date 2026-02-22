package handler

import "testing"

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
