package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// GetMetadata retrieves a value from the feed_metadata table.
func (db *DB) GetMetadata(ctx context.Context, key string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM feed_metadata WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetMetadata stores a key-value pair in the feed_metadata table.
func (db *DB) SetMetadata(ctx context.Context, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT OR REPLACE INTO feed_metadata (key, value) VALUES (?, ?)`,
		key, value)
	return err
}

// NearbyStopRow represents a stop with its distance from a query point.
type NearbyStopRow struct {
	StopID             string
	StopCode           string
	StopName           string
	StopLat            float64
	StopLon            float64
	LocationType       int
	WheelchairBoarding int
	DistanceMeters     float64 // Computed after query via Haversine
}

// NearbyStops finds stops within a bounding box using the R-Tree index.
// The caller should refine distances with Haversine and re-sort.
func (db *DB) NearbyStops(ctx context.Context, lat, lon, radiusDeg float64, limit int) ([]NearbyStopRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.stop_id, s.stop_code, s.stop_name, s.stop_lat, s.stop_lon,
		       s.location_type, s.wheelchair_boarding
		FROM stops_rtree AS r
		JOIN stops AS s ON s.rowid = r.id
		WHERE r.min_lat >= ? AND r.max_lat <= ?
		  AND r.min_lon >= ? AND r.max_lon <= ?
		ORDER BY (s.stop_lat - ?)*(s.stop_lat - ?) + (s.stop_lon - ?)*(s.stop_lon - ?)
		LIMIT ?`,
		lat-radiusDeg, lat+radiusDeg,
		lon-radiusDeg, lon+radiusDeg,
		lat, lat, lon, lon,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("nearby stops query: %w", err)
	}
	defer rows.Close()

	var stops []NearbyStopRow
	for rows.Next() {
		var s NearbyStopRow
		if err := rows.Scan(&s.StopID, &s.StopCode, &s.StopName, &s.StopLat, &s.StopLon,
			&s.LocationType, &s.WheelchairBoarding); err != nil {
			return nil, fmt.Errorf("scan stop: %w", err)
		}
		stops = append(stops, s)
	}
	return stops, rows.Err()
}

// DepartureRow represents a scheduled departure at a stop.
type DepartureRow struct {
	TripID        string
	RouteID       string
	RouteShort    string
	RouteLong     string
	RouteColor    string
	RouteType     int
	TripHeadsign  string
	DirectionID   int
	DepartureTime string // HH:MM:SS format (can exceed 24:00:00 for next-day trips)
	StopSequence  int
}

// DeparturesForStop returns upcoming scheduled departures for a stop on a given date.
// The date is used to filter by active service (calendar + calendar_dates).
// afterTime is in HH:MM:SS format.
func (db *DB) DeparturesForStop(ctx context.Context, stopID string, date time.Time, afterTime string, limit int) ([]DepartureRow, error) {
	dateStr := date.Format("20060102")
	dayCol := dayColumn(date.Weekday())

	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT st.trip_id, t.route_id, r.route_short_name, r.route_long_name,
		       r.route_color, r.route_type, t.trip_headsign, t.direction_id,
		       st.departure_time, st.stop_sequence
		FROM stop_times st
		JOIN trips t ON t.trip_id = st.trip_id
		JOIN routes r ON r.route_id = t.route_id
		WHERE st.stop_id = ?
		  AND st.departure_time >= ?
		  AND (
		    (t.service_id IN (
		      SELECT service_id FROM calendar
		      WHERE %s = 1 AND start_date <= ? AND end_date >= ?
		    ) AND t.service_id NOT IN (
		      SELECT service_id FROM calendar_dates
		      WHERE date = ? AND exception_type = 2
		    ))
		    OR t.service_id IN (
		      SELECT service_id FROM calendar_dates
		      WHERE date = ? AND exception_type = 1
		    )
		  )
		ORDER BY st.departure_time
		LIMIT ?`, dayCol),
		stopID, afterTime,
		dateStr, dateStr,
		dateStr,
		dateStr,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("departures query: %w", err)
	}
	defer rows.Close()

	var deps []DepartureRow
	for rows.Next() {
		var d DepartureRow
		if err := rows.Scan(&d.TripID, &d.RouteID, &d.RouteShort, &d.RouteLong,
			&d.RouteColor, &d.RouteType, &d.TripHeadsign, &d.DirectionID,
			&d.DepartureTime, &d.StopSequence); err != nil {
			return nil, fmt.Errorf("scan departure: %w", err)
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

// AllDeparturesForStopRoute returns all departures today for a specific route/direction at a stop.
// Used for computing service intervals ("Every 20 minutes").
func (db *DB) AllDeparturesForStopRoute(ctx context.Context, stopID, routeID string, directionID int, date time.Time) ([]string, error) {
	dateStr := date.Format("20060102")
	dayCol := dayColumn(date.Weekday())

	rows, err := db.QueryContext(ctx, fmt.Sprintf(`
		SELECT st.departure_time
		FROM stop_times st
		JOIN trips t ON t.trip_id = st.trip_id
		WHERE st.stop_id = ?
		  AND t.route_id = ?
		  AND t.direction_id = ?
		  AND (
		    (t.service_id IN (
		      SELECT service_id FROM calendar
		      WHERE %s = 1 AND start_date <= ? AND end_date >= ?
		    ) AND t.service_id NOT IN (
		      SELECT service_id FROM calendar_dates
		      WHERE date = ? AND exception_type = 2
		    ))
		    OR t.service_id IN (
		      SELECT service_id FROM calendar_dates
		      WHERE date = ? AND exception_type = 1
		    )
		  )
		ORDER BY st.departure_time`, dayCol),
		stopID, routeID, directionID,
		dateStr, dateStr,
		dateStr,
		dateStr,
	)
	if err != nil {
		return nil, fmt.Errorf("all departures query: %w", err)
	}
	defer rows.Close()

	var times []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan time: %w", err)
		}
		times = append(times, t)
	}
	return times, rows.Err()
}

// StopsForRoute returns all stops on a route in a given direction, ordered by stop_sequence.
func (db *DB) StopsForRoute(ctx context.Context, routeID string, directionID int, date time.Time) ([]StopOnRoute, error) {
	dateStr := date.Format("20060102")
	dayCol := dayColumn(date.Weekday())

	// Get a representative trip for this route/direction on this date
	var tripID string
	err := db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT t.trip_id
		FROM trips t
		WHERE t.route_id = ?
		  AND t.direction_id = ?
		  AND (
		    (t.service_id IN (
		      SELECT service_id FROM calendar
		      WHERE %s = 1 AND start_date <= ? AND end_date >= ?
		    ) AND t.service_id NOT IN (
		      SELECT service_id FROM calendar_dates
		      WHERE date = ? AND exception_type = 2
		    ))
		    OR t.service_id IN (
		      SELECT service_id FROM calendar_dates
		      WHERE date = ? AND exception_type = 1
		    )
		  )
		LIMIT 1`, dayCol),
		routeID, directionID,
		dateStr, dateStr,
		dateStr,
		dateStr,
	).Scan(&tripID)
	if err != nil {
		return nil, fmt.Errorf("find representative trip: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT s.stop_id, s.stop_name, s.stop_lat, s.stop_lon, st.stop_sequence
		FROM stop_times st
		JOIN stops s ON s.stop_id = st.stop_id
		WHERE st.trip_id = ?
		ORDER BY st.stop_sequence`,
		tripID,
	)
	if err != nil {
		return nil, fmt.Errorf("stops for route query: %w", err)
	}
	defer rows.Close()

	var stops []StopOnRoute
	for rows.Next() {
		var s StopOnRoute
		if err := rows.Scan(&s.StopID, &s.StopName, &s.StopLat, &s.StopLon, &s.StopSequence); err != nil {
			return nil, fmt.Errorf("scan stop on route: %w", err)
		}
		stops = append(stops, s)
	}
	return stops, rows.Err()
}

// StopOnRoute represents a stop along a specific route.
type StopOnRoute struct {
	StopID       string
	StopName     string
	StopLat      float64
	StopLon      float64
	StopSequence int
}

// AllRoutes returns all routes ordered by sort order then route short name.
func (db *DB) AllRoutes(ctx context.Context) ([]RouteRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT route_id, route_short_name, route_long_name, route_type,
		       route_color, route_text_color
		FROM routes
		ORDER BY route_sort_order, route_short_name`)
	if err != nil {
		return nil, fmt.Errorf("all routes query: %w", err)
	}
	defer rows.Close()

	var routes []RouteRow
	for rows.Next() {
		var r RouteRow
		if err := rows.Scan(&r.RouteID, &r.RouteShort, &r.RouteLong, &r.RouteType,
			&r.RouteColor, &r.RouteTextColor); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, r)
	}
	return routes, rows.Err()
}

// RouteRow represents a transit route.
type RouteRow struct {
	RouteID        string
	RouteShort     string
	RouteLong      string
	RouteType      int
	RouteColor     string
	RouteTextColor string
}

// HasData returns true if the database has GTFS data imported.
func (db *DB) HasData(ctx context.Context) bool {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM routes`).Scan(&count)
	return err == nil && count > 0
}

// RebuildRTree repopulates the R-Tree index from the stops table.
func (db *DB) RebuildRTree(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM stops_rtree`); err != nil {
		return fmt.Errorf("clear rtree: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO stops_rtree(id, min_lat, max_lat, min_lon, max_lon)
		 SELECT rowid, stop_lat, stop_lat, stop_lon, stop_lon FROM stops`); err != nil {
		return fmt.Errorf("populate rtree: %w", err)
	}
	return nil
}

// dayColumn returns the SQLite column name for a given weekday.
func dayColumn(d time.Weekday) string {
	switch d {
	case time.Monday:
		return "monday"
	case time.Tuesday:
		return "tuesday"
	case time.Wednesday:
		return "wednesday"
	case time.Thursday:
		return "thursday"
	case time.Friday:
		return "friday"
	case time.Saturday:
		return "saturday"
	case time.Sunday:
		return "sunday"
	default:
		return "monday"
	}
}
