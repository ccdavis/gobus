package handler

import (
	"testing"
	"time"
)

func TestParseGTFSTime(t *testing.T) {
	base := time.Date(2025, 6, 15, 0, 0, 0, 0, time.Local)

	tests := []struct {
		name     string
		input    string
		wantHour int
		wantMin  int
	}{
		{"morning", "08:30:00", 8, 30},
		{"noon", "12:00:00", 12, 0},
		{"evening", "18:45:00", 18, 45},
		{"midnight boundary", "24:00:00", 0, 0}, // next day midnight
		{"after midnight", "25:30:00", 1, 30},   // 1:30 AM next service day
		{"late night", "26:15:00", 2, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGTFSTime(tt.input, base)
			if got.Hour() != tt.wantHour || got.Minute() != tt.wantMin {
				t.Errorf("parseGTFSTime(%q) = %s, want %02d:%02d",
					tt.input, got.Format("15:04"), tt.wantHour, tt.wantMin)
			}
			// All results should be on the same date as base (or next day for >24h)
			if got.Year() != base.Year() || got.Month() != base.Month() {
				t.Errorf("parseGTFSTime(%q) wrong date: %s", tt.input, got.Format("2006-01-02"))
			}
		})
	}
}

func TestParseGTFSTime_Ordering(t *testing.T) {
	base := time.Date(2025, 6, 15, 0, 0, 0, 0, time.Local)

	// 23:00 < 24:30 < 25:00 (in GTFS, these are ordered)
	t1 := parseGTFSTime("23:00:00", base)
	t2 := parseGTFSTime("24:30:00", base)
	t3 := parseGTFSTime("25:00:00", base)

	if !t1.Before(t2) {
		t.Errorf("23:00 should be before 24:30")
	}
	if !t2.Before(t3) {
		t.Errorf("24:30 should be before 25:00")
	}
}

func TestAbs(t *testing.T) {
	tests := []struct {
		input, want int
	}{
		{0, 0},
		{5, 5},
		{-5, 5},
		{-1, 1},
	}
	for _, tt := range tests {
		if got := abs(tt.input); got != tt.want {
			t.Errorf("abs(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
