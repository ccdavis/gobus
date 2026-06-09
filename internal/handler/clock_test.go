package handler

import (
	"testing"
	"time"
)

func TestAgencyLocation(t *testing.T) {
	if got := agencyLoc.String(); got != "America/Chicago" {
		t.Fatalf("agencyLoc = %q, want America/Chicago (embedded tzdata not loading?)", got)
	}

	// The zone must be DST-aware: CST (-6h) in January, CDT (-5h) in July.
	_, janOff := time.Date(2025, 1, 15, 12, 0, 0, 0, agencyLoc).Zone()
	_, julOff := time.Date(2025, 7, 15, 12, 0, 0, 0, agencyLoc).Zone()
	if janOff != -6*3600 {
		t.Errorf("January offset = %d, want -21600 (CST)", janOff)
	}
	if julOff != -5*3600 {
		t.Errorf("July offset = %d, want -18000 (CDT)", julOff)
	}
}

func TestAgencyNowIsAgencyZone(t *testing.T) {
	// agencyNow must report the agency zone regardless of the host's local zone.
	if loc := agencyNow().Location(); loc != agencyLoc {
		t.Errorf("agencyNow().Location() = %v, want %v", loc, agencyLoc)
	}
}
