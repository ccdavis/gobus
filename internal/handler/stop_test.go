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

// minutesUntil is the test shorthand for the two-step conversion the handlers
// do: GTFS time on now's service date → instant → minutes from now.
func minutesUntil(gtfsTime string, now time.Time) int {
	return minutesUntilInstant(gtfsInstant(gtfsTime, now), now)
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

// TestMinutesUntil_DST pins the daylight-saving behavior that the old
// "midnight + duration" math got wrong (the schedules-off-by-an-hour bug seen
// on the March transition). Anchored to America/Chicago so it's deterministic
// regardless of the host's local zone.
func TestMinutesUntil_DST(t *testing.T) {
	central, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skip("America/Chicago tzdata unavailable")
	}

	// Spring forward 2025: 2:00 AM CST jumps to 3:00 AM CDT on March 9. After
	// the jump, the old code reported these an hour too high (e.g. 120 not 60).
	spring := time.Date(2025, 3, 9, 14, 0, 0, 0, central) // 2:00 PM CDT
	for _, tt := range []struct {
		gtfsTime string
		want     int
	}{
		{"14:30:00", 30},
		{"15:00:00", 60},
		{"23:00:00", 540}, // 9 hours later, same day
	} {
		if got := minutesUntil(tt.gtfsTime, spring); got != tt.want {
			t.Errorf("spring-forward minutesUntil(%q) = %d, want %d", tt.gtfsTime, got, tt.want)
		}
	}

	// Fall back 2025: 2:00 AM CDT returns to 1:00 AM CST on November 2.
	fall := time.Date(2025, 11, 2, 14, 0, 0, 0, central) // 2:00 PM CST
	if got := minutesUntil("15:00:00", fall); got != 60 {
		t.Errorf("fall-back minutesUntil(15:00) = %d, want 60", got)
	}

	// After-midnight GTFS time (24:30) resolves to 12:30 AM the next day.
	eve := time.Date(2025, 3, 8, 23, 50, 0, 0, central)
	if got := minutesUntil("24:30:00", eve); got != 40 {
		t.Errorf("after-midnight minutesUntil(24:30) = %d, want 40", got)
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
