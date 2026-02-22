package handler

import (
	"testing"
	"time"
)

func TestFormatGTFSTime(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"08:30:00", "8:30 AM"},
		{"00:00:00", "12:00 AM"},
		{"12:00:00", "12:00 PM"},
		{"12:30:00", "12:30 PM"},
		{"13:00:00", "1:00 PM"},
		{"23:59:00", "11:59 PM"},
		// GTFS times >24h (next service day)
		{"24:00:00", "12:00 AM"},
		{"25:30:00", "1:30 AM"},
		{"26:15:00", "2:15 AM"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := formatGTFSTime(tt.input)
			if got != tt.want {
				t.Errorf("formatGTFSTime(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMinutesUntil(t *testing.T) {
	now := time.Date(2025, 6, 15, 14, 0, 0, 0, time.Local) // 2:00 PM

	tests := []struct {
		name     string
		gtfsTime string
		want     int
	}{
		{"30 min from now", "14:30:00", 30},
		{"exactly now", "14:00:00", 0},
		{"1 min from now", "14:01:00", 1},
		{"in the past returns 0", "13:00:00", 0},
		{"end of day", "23:59:00", 599},
		// GTFS >24h time
		{"next day 1am", "25:00:00", 660}, // 11 hours = 660 min
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minutesUntil(tt.gtfsTime, now)
			if got != tt.want {
				t.Errorf("minutesUntil(%q, 14:00) = %d, want %d", tt.gtfsTime, got, tt.want)
			}
		})
	}
}

func TestExpandDirectionText(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"NB", "Northbound"},
		{"SB", "Southbound"},
		{"EB", "Eastbound"},
		{"WB", "Westbound"},
		{"", ""},
		{"Northbound", "Northbound"}, // already expanded
		{"Loop", "Loop"},             // unknown
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandDirectionText(tt.input)
			if got != tt.want {
				t.Errorf("expandDirectionText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
