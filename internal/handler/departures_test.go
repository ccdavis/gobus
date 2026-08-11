package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gobus/internal/config"
	"gobus/internal/geocode"
	"gobus/internal/nextrip"
	"gobus/internal/realtime"
	"gobus/internal/storage"
)

// testHandler builds a Handler over a real (temp-file) SQLite database and a
// NexTrip client pointed at ntURL (use an httptest server, or an unroutable
// URL to exercise the schedule-only fallback).
func testHandler(t *testing.T, ntURL string) (*Handler, *storage.DB) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"), logger)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	nt := nextrip.NewClient(ntURL, logger)
	cfg := &config.Config{GeocodeEnabled: false}
	h := New(db, nt, realtime.NewStore(), geocode.New("gobus-test"), cfg, logger)
	return h, db
}

func mustExec(t *testing.T, db *storage.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// seedStop inserts a stop and its R-Tree entry.
func seedStop(t *testing.T, db *storage.DB, id, name string, lat, lon float64) {
	t.Helper()
	mustExec(t, db, `INSERT INTO stops (stop_id, stop_code, stop_name, stop_desc, stop_lat, stop_lon, location_type, wheelchair_boarding)
		VALUES (?, ?, ?, '', ?, ?, 0, 0)`, id, id, name, lat, lon)
	mustExec(t, db, `INSERT INTO stops_rtree (id, min_lat, max_lat, min_lon, max_lon)
		SELECT rowid, stop_lat, stop_lat, stop_lon, stop_lon FROM stops WHERE stop_id = ?`, id)
}

func seedRoute(t *testing.T, db *storage.DB, id, short string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO routes (route_id, route_short_name, route_long_name, route_type, route_color, route_text_color, route_sort_order)
		VALUES (?, ?, ?, 3, 'FF0000', '000000', 1)`, id, short, "Route "+short)
}

// seedCalendarDay inserts a service active only on the weekday of `day`,
// valid for a year around it.
func seedCalendarDay(t *testing.T, db *storage.DB, serviceID string, day time.Time) {
	t.Helper()
	cols := map[time.Weekday]string{
		time.Monday: "monday", time.Tuesday: "tuesday", time.Wednesday: "wednesday",
		time.Thursday: "thursday", time.Friday: "friday",
		time.Saturday: "saturday", time.Sunday: "sunday",
	}
	col := cols[day.Weekday()]
	mustExec(t, db, fmt.Sprintf(`INSERT INTO calendar (service_id, monday, tuesday, wednesday, thursday, friday, saturday, sunday, start_date, end_date)
		VALUES (?, 0, 0, 0, 0, 0, 0, 0, ?, ?)`),
		serviceID, day.AddDate(0, 0, -180).Format("20060102"), day.AddDate(0, 0, 180).Format("20060102"))
	mustExec(t, db, fmt.Sprintf(`UPDATE calendar SET %s = 1 WHERE service_id = ?`, col), serviceID)
}

func seedTrip(t *testing.T, db *storage.DB, tripID, routeID, serviceID, headsign string, directionID int) {
	t.Helper()
	mustExec(t, db, `INSERT INTO trips (trip_id, route_id, service_id, trip_headsign, direction_id, block_id, shape_id)
		VALUES (?, ?, ?, ?, ?, '', '')`, tripID, routeID, serviceID, headsign, directionID)
}

func seedStopTime(t *testing.T, db *storage.DB, tripID, stopID, depTime string, seq int) {
	t.Helper()
	mustExec(t, db, `INSERT INTO stop_times (trip_id, arrival_time, departure_time, stop_id, stop_sequence, pickup_type, drop_off_type)
		VALUES (?, ?, ?, ?, ?, 0, 0)`, tripID, depTime, depTime, stopID, seq)
}

// emptyNexTripServer returns a NexTrip stub that always answers with no
// realtime departures.
func emptyNexTripServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"stops":[],"alerts":[],"departures":[]}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestScheduledDeparturesForStop_PostMidnight is the regression test for the
// GTFS service-day bug: shortly after midnight, the previous service day's
// over-24h trips are the imminent ones, and the current day's over-24h trips
// are actually tomorrow.
func TestScheduledDeparturesForStop_PostMidnight(t *testing.T) {
	h, db := testHandler(t, "http://127.0.0.1:0")

	// Monday 2026-08-10 00:30 agency time.
	now := time.Date(2026, 8, 10, 0, 30, 0, 0, agencyLoc)
	sunday := now.AddDate(0, 0, -1)

	seedRoute(t, db, "R1", "10")
	seedStop(t, db, "S1", "1st St & Main Ave", 44.9778, -93.2650)
	seedCalendarDay(t, db, "SUN", sunday)
	seedCalendarDay(t, db, "MON", now)

	// Sunday service, 25:00:00 = Monday 1:00 AM — departs in 30 minutes.
	seedTrip(t, db, "T_SUN", "R1", "SUN", "Downtown", 0)
	seedStopTime(t, db, "T_SUN", "S1", "25:00:00", 1)
	// Monday service, 25:00:00 = Tuesday 1:00 AM — 24.5 hours away.
	seedTrip(t, db, "T_MON_LATE", "R1", "MON", "Downtown", 0)
	seedStopTime(t, db, "T_MON_LATE", "S1", "25:00:00", 1)
	// Monday service, 05:30:00 — 5 hours away.
	seedTrip(t, db, "T_MON_AM", "R1", "MON", "Downtown", 0)
	seedStopTime(t, db, "T_MON_AM", "S1", "05:30:00", 1)

	deps := h.scheduledDeparturesForStop(context.Background(), "S1", now, 10)
	if len(deps) != 3 {
		t.Fatalf("got %d departures, want 3: %+v", len(deps), deps)
	}

	wantOrder := []string{"T_SUN", "T_MON_AM", "T_MON_LATE"}
	for i, want := range wantOrder {
		if deps[i].TripID != want {
			t.Errorf("departure[%d] = %s, want %s", i, deps[i].TripID, want)
		}
	}

	if mins := minutesUntilInstant(deps[0].Instant, now); mins != 30 {
		t.Errorf("Sunday 25:00:00 trip is %d min away at Monday 00:30, want 30", mins)
	}
	if mins := minutesUntilInstant(deps[2].Instant, now); mins != 24*60+30 {
		t.Errorf("Monday 25:00:00 trip is %d min away, want %d (Tuesday 1 AM)", mins, 24*60+30)
	}
}

// TestScheduledDeparturesForStop_CalendarDates covers calendar_dates
// exceptions: a removed service day must not produce departures, an added one
// must.
func TestScheduledDeparturesForStop_CalendarDates(t *testing.T) {
	h, db := testHandler(t, "http://127.0.0.1:0")

	now := time.Date(2026, 8, 10, 9, 0, 0, 0, agencyLoc) // Monday 9:00 AM
	dateStr := now.Format("20060102")

	seedRoute(t, db, "R1", "10")
	seedStop(t, db, "S1", "1st St & Main Ave", 44.9778, -93.2650)
	seedCalendarDay(t, db, "MON", now)

	// Normal Monday trip, but today is removed via exception_type=2.
	seedTrip(t, db, "T_REMOVED", "R1", "MON", "Downtown", 0)
	seedStopTime(t, db, "T_REMOVED", "S1", "10:00:00", 1)
	mustExec(t, db, `INSERT INTO calendar_dates (service_id, date, exception_type) VALUES ('MON', ?, 2)`, dateStr)

	// Special service added only for today via exception_type=1.
	mustExec(t, db, `INSERT INTO calendar (service_id, monday, tuesday, wednesday, thursday, friday, saturday, sunday, start_date, end_date)
		VALUES ('SPECIAL', 0, 0, 0, 0, 0, 0, 0, '19700101', '19700101')`)
	mustExec(t, db, `INSERT INTO calendar_dates (service_id, date, exception_type) VALUES ('SPECIAL', ?, 1)`, dateStr)
	seedTrip(t, db, "T_ADDED", "R1", "SPECIAL", "Downtown", 0)
	seedStopTime(t, db, "T_ADDED", "S1", "11:00:00", 1)

	deps := h.scheduledDeparturesForStop(context.Background(), "S1", now, 10)
	if len(deps) != 1 || deps[0].TripID != "T_ADDED" {
		t.Fatalf("got %+v, want exactly T_ADDED", deps)
	}
}

// TestNearbyRoutes_LaterTimesFromSameStopOnly is the regression test for the
// wrong-stop attribution bug: a route+direction seen at two nearby stops must
// not present the second stop's departure as a "later time" at the first stop.
func TestNearbyRoutes_LaterTimesFromSameStopOnly(t *testing.T) {
	ntSrv := emptyNexTripServer(t)
	h, db := testHandler(t, ntSrv.URL)

	now := agencyNow()
	// All times relative to the real clock (the HTTP handler uses agencyNow).
	dep := func(minsAhead int) string {
		tt := now.Add(time.Duration(minsAhead) * time.Minute)
		return tt.Format("15:04:05")
	}
	if now.Hour() >= 22 {
		t.Skip("test schedule would cross midnight; times are same-service-day")
	}

	seedRoute(t, db, "R1", "10")
	// Stop A near the query point; stop B ~200m farther on the same route.
	seedStop(t, db, "A", "Near Stop", 44.9778, -93.2650)
	seedStop(t, db, "B", "Far Stop", 44.9796, -93.2650)
	seedCalendarDay(t, db, "EVERYDAY", now)
	mustExec(t, db, `UPDATE calendar SET monday=1,tuesday=1,wednesday=1,thursday=1,friday=1,saturday=1,sunday=1 WHERE service_id='EVERYDAY'`)

	// Stop A: departures in 10 and 40 minutes. Stop B: in 20 minutes.
	seedTrip(t, db, "TA1", "R1", "EVERYDAY", "Downtown", 0)
	seedStopTime(t, db, "TA1", "A", dep(10), 1)
	seedTrip(t, db, "TA2", "R1", "EVERYDAY", "Downtown", 0)
	seedStopTime(t, db, "TA2", "A", dep(40), 1)
	seedTrip(t, db, "TB1", "R1", "EVERYDAY", "Downtown", 0)
	seedStopTime(t, db, "TB1", "B", dep(20), 1)

	req := httptest.NewRequest("GET", "/nearby?view=routes&lat=44.9778&lon=-93.2650", nil)
	rec := httptest.NewRecorder()
	h.Nearby(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "Near Stop") {
		t.Fatalf("expected route row at Near Stop; body:\n%s", body)
	}
	// B's departure time must not appear anywhere: it is neither the primary
	// row (A is nearer) nor a legitimate "later time" at A.
	bTime := formatGTFSTime(dep(20))
	if strings.Contains(body, bTime) {
		t.Errorf("stop B's departure %q shown in nearby routes view (wrong-stop attribution)", bTime)
	}
	// A's own later time should be there.
	aLater := formatGTFSTime(dep(40))
	if !strings.Contains(body, aLater) {
		t.Errorf("stop A's later departure %q missing from nearby routes view", aLater)
	}
}

// TestNearby_InitialEmptyRadiusAdvances is the regression test for the
// auto-advance bug: an initial search (offset 0) with no stops in the first
// tier must widen the radius instead of reporting nothing nearby.
func TestNearby_InitialEmptyRadiusAdvances(t *testing.T) {
	ntSrv := emptyNexTripServer(t)
	h, db := testHandler(t, ntSrv.URL)

	now := agencyNow()
	if now.Hour() >= 22 {
		t.Skip("test schedule would cross midnight; times are same-service-day")
	}

	seedRoute(t, db, "R1", "10")
	// Stop ~2 km north of the query point: outside the 450/900/1800 m tiers,
	// inside 3600 m.
	seedStop(t, db, "FAR", "Distant Stop", 44.9958, -93.2650)
	seedCalendarDay(t, db, "EVERYDAY", now)
	mustExec(t, db, `UPDATE calendar SET monday=1,tuesday=1,wednesday=1,thursday=1,friday=1,saturday=1,sunday=1 WHERE service_id='EVERYDAY'`)
	seedTrip(t, db, "T1", "R1", "EVERYDAY", "Downtown", 0)
	seedStopTime(t, db, "T1", "FAR", now.Add(15*time.Minute).Format("15:04:05"), 1)

	req := httptest.NewRequest("GET", "/nearby?view=stops&lat=44.9778&lon=-93.2650", nil)
	rec := httptest.NewRecorder()
	h.Nearby(rec, req)

	if !strings.Contains(rec.Body.String(), "Distant Stop") {
		t.Errorf("initial empty search did not advance radius tiers to find the stop")
	}
}

// TestNearby_SlowRealtimeBounded asserts that slow NexTrip responses are
// fetched concurrently and cannot stack serially into a multi-minute page.
func TestNearby_SlowRealtimeBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	const delay = 1 * time.Second
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"stops":[],"alerts":[],"departures":[]}`)
	}))
	t.Cleanup(slow.Close)

	h, db := testHandler(t, slow.URL)
	now := agencyNow()
	if now.Hour() >= 22 {
		t.Skip("test schedule would cross midnight; times are same-service-day")
	}

	seedRoute(t, db, "R1", "10")
	seedCalendarDay(t, db, "EVERYDAY", now)
	mustExec(t, db, `UPDATE calendar SET monday=1,tuesday=1,wednesday=1,thursday=1,friday=1,saturday=1,sunday=1 WHERE service_id='EVERYDAY'`)

	// Six stops in the first tier, each served by its own trip.
	for i := 0; i < 6; i++ {
		stopID := fmt.Sprintf("S%d", i)
		seedStop(t, db, stopID, fmt.Sprintf("Stop %d", i), 44.9778+float64(i)*0.0004, -93.2650)
		tripID := fmt.Sprintf("T%d", i)
		seedTrip(t, db, tripID, "R1", "EVERYDAY", fmt.Sprintf("Headsign %d", i), 0)
		seedStopTime(t, db, tripID, stopID, now.Add(time.Duration(10+i)*time.Minute).Format("15:04:05"), 1)
	}

	req := httptest.NewRequest("GET", "/nearby?view=routes&lat=44.9778&lon=-93.2650", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	h.Nearby(rec, req)
	elapsed := time.Since(start)

	// Serial fetching would take ≥ 6 × delay; concurrent fetching one round.
	if elapsed > 4*delay {
		t.Errorf("nearby page took %v with 6 slow realtime lookups; serial fetching suspected", elapsed)
	}
	if !strings.Contains(rec.Body.String(), "Stop 0") {
		t.Errorf("schedule data missing from page despite slow realtime")
	}
}
