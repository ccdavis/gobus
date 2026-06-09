package handler

import (
	"time"
	// Embed the IANA timezone database so America/Chicago resolves in every
	// build that includes this package — desktop, tests, and the gomobile
	// bind — without relying on OS zoneinfo.
	_ "time/tzdata"
)

// agencyLoc is the transit agency's timezone (America/Chicago for Metro
// Transit). GTFS departure times and NexTrip predictions are in this zone, so
// every schedule comparison must be done in agency-local time rather than the
// host machine's local zone — otherwise a device set to another timezone shows
// wrong departures year-round. With tzdata embedded above, LoadLocation
// succeeds without OS zoneinfo; the FixedZone fallback is a last resort and
// intentionally has no DST.
var agencyLoc = func() *time.Location {
	if loc, err := time.LoadLocation("America/Chicago"); err == nil {
		return loc
	}
	return time.FixedZone("CST", -6*60*60)
}()

// agencyNow returns the current instant expressed in the agency's timezone.
func agencyNow() time.Time {
	return time.Now().In(agencyLoc)
}
