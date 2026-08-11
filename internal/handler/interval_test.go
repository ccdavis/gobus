package handler

import (
	"testing"
	"time"
)

func TestGTFSInstant(t *testing.T) {
	base := time.Date(2025, 6, 15, 0, 0, 0, 0, agencyLoc)

	tests := []struct {
		name     string
		input    string
		wantDay  int
		wantHour int
		wantMin  int
	}{
		{"morning", "08:30:00", 15, 8, 30},
		{"noon", "12:00:00", 15, 12, 0},
		{"evening", "18:45:00", 15, 18, 45},
		{"before midnight", "23:30:00", 15, 23, 30},
		{"midnight boundary", "24:00:00", 16, 0, 0}, // next calendar day
		{"after midnight", "24:30:00", 16, 0, 30},
		{"1:30 AM next day", "25:30:00", 16, 1, 30},
		{"late night", "26:15:00", 16, 2, 15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gtfsInstant(tt.input, base)
			if got.Day() != tt.wantDay || got.Hour() != tt.wantHour || got.Minute() != tt.wantMin {
				t.Errorf("gtfsInstant(%q, %s) = %s, want day %d %02d:%02d",
					tt.input, base.Format("2006-01-02"), got.Format("2006-01-02 15:04"),
					tt.wantDay, tt.wantHour, tt.wantMin)
			}
		})
	}
}

func TestGTFSInstant_Ordering(t *testing.T) {
	base := time.Date(2025, 6, 15, 0, 0, 0, 0, agencyLoc)

	// 23:00 < 24:30 < 25:00 (in GTFS, these are ordered)
	t1 := gtfsInstant("23:00:00", base)
	t2 := gtfsInstant("24:30:00", base)
	t3 := gtfsInstant("25:00:00", base)

	if !t1.Before(t2) {
		t.Errorf("23:00 should be before 24:30")
	}
	if !t2.Before(t3) {
		t.Errorf("24:30 should be before 25:00")
	}
}

func TestGTFSInstant_ServiceDateVsCalendarDate(t *testing.T) {
	// A 25:00:00 trip on Sunday's service date is Monday 1:00 AM.
	sunday := time.Date(2025, 6, 15, 0, 0, 0, 0, agencyLoc)
	got := gtfsInstant("25:00:00", sunday)
	want := time.Date(2025, 6, 16, 1, 0, 0, 0, agencyLoc)
	if !got.Equal(want) {
		t.Errorf("gtfsInstant(25:00:00, Sunday) = %s, want %s", got, want)
	}

	// The same clock string on Monday's service date is Tuesday 1:00 AM —
	// the service date, not "now", must drive the calendar date.
	monday := sunday.AddDate(0, 0, 1)
	got = gtfsInstant("25:00:00", monday)
	want = time.Date(2025, 6, 17, 1, 0, 0, 0, agencyLoc)
	if !got.Equal(want) {
		t.Errorf("gtfsInstant(25:00:00, Monday) = %s, want %s", got, want)
	}
}

func TestGTFSInstant_DST(t *testing.T) {
	// Spring forward: 2025-03-09, 2:00 AM CST jumps to 3:00 AM CDT.
	// A Saturday-service 26:30:00 trip is Sunday "2:30 AM", which doesn't
	// exist; time.Date normalizes it into CDT. The key property: the zone
	// offset must be the post-transition one, not midnight's.
	saturday := time.Date(2025, 3, 8, 0, 0, 0, 0, agencyLoc)
	got := gtfsInstant("27:30:00", saturday) // Sunday 3:30 AM CDT
	want := time.Date(2025, 3, 9, 3, 30, 0, 0, agencyLoc)
	if !got.Equal(want) {
		t.Errorf("spring-forward gtfsInstant(27:30:00) = %s, want %s", got, want)
	}
	if _, off := got.Zone(); off != -5*3600 {
		t.Errorf("spring-forward offset = %d, want CDT (-18000)", off)
	}

	// Fall back: 2025-11-02, 2:00 AM CDT returns to 1:00 AM CST.
	// A Saturday-service 27:00:00 trip is Sunday 3:00 AM CST.
	saturday = time.Date(2025, 11, 1, 0, 0, 0, 0, agencyLoc)
	got = gtfsInstant("27:00:00", saturday)
	want = time.Date(2025, 11, 2, 3, 0, 0, 0, agencyLoc)
	if !got.Equal(want) {
		t.Errorf("fall-back gtfsInstant(27:00:00) = %s, want %s", got, want)
	}
	if _, off := got.Zone(); off != -6*3600 {
		t.Errorf("fall-back offset = %d, want CST (-21600)", off)
	}
}

func TestMinutesUntilInstant(t *testing.T) {
	now := time.Date(2025, 6, 16, 0, 30, 0, 0, agencyLoc) // Monday 12:30 AM

	// Sunday-service 25:00:00 = Monday 1:00 AM = 30 minutes away.
	sunday := now.AddDate(0, 0, -1)
	if got := minutesUntilInstant(gtfsInstant("25:00:00", sunday), now); got != 30 {
		t.Errorf("Sunday 25:00:00 at Monday 00:30 = %d min away, want 30", got)
	}

	// Monday-service 25:00:00 = Tuesday 1:00 AM = 24.5 hours away — this is
	// the departure the pre-fix code showed as 30 minutes away.
	if got := minutesUntilInstant(gtfsInstant("25:00:00", now), now); got != 24*60+30 {
		t.Errorf("Monday 25:00:00 at Monday 00:30 = %d min away, want %d", got, 24*60+30)
	}

	// Past instants floor at zero.
	if got := minutesUntilInstant(now.Add(-5*time.Minute), now); got != 0 {
		t.Errorf("past instant = %d, want 0", got)
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
