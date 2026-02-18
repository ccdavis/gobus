package storage

import "fmt"

// migrate creates the GTFS schema if it doesn't exist.
func (db *DB) migrate() error {
	for i, stmt := range migrations {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration %d: %w", i, err)
		}
	}
	db.logger.Info("database migrations applied")
	return nil
}

var migrations = []string{
	// Agency
	`CREATE TABLE IF NOT EXISTS agency (
		agency_id   TEXT PRIMARY KEY,
		agency_name TEXT NOT NULL,
		agency_url  TEXT NOT NULL DEFAULT '',
		agency_timezone TEXT NOT NULL DEFAULT 'America/Chicago'
	)`,

	// Routes
	`CREATE TABLE IF NOT EXISTS routes (
		route_id         TEXT PRIMARY KEY,
		agency_id        TEXT REFERENCES agency(agency_id),
		route_short_name TEXT,
		route_long_name  TEXT,
		route_type       INTEGER NOT NULL DEFAULT 3,
		route_color      TEXT,
		route_text_color TEXT,
		route_sort_order INTEGER
	)`,

	// Stops
	`CREATE TABLE IF NOT EXISTS stops (
		stop_id            TEXT PRIMARY KEY,
		stop_code          TEXT,
		stop_name          TEXT NOT NULL,
		stop_desc          TEXT,
		stop_lat           REAL NOT NULL,
		stop_lon           REAL NOT NULL,
		zone_id            TEXT,
		stop_url           TEXT,
		location_type      INTEGER DEFAULT 0,
		parent_station     TEXT,
		wheelchair_boarding INTEGER DEFAULT 0
	)`,

	// Calendar
	`CREATE TABLE IF NOT EXISTS calendar (
		service_id TEXT PRIMARY KEY,
		monday     INTEGER NOT NULL DEFAULT 0,
		tuesday    INTEGER NOT NULL DEFAULT 0,
		wednesday  INTEGER NOT NULL DEFAULT 0,
		thursday   INTEGER NOT NULL DEFAULT 0,
		friday     INTEGER NOT NULL DEFAULT 0,
		saturday   INTEGER NOT NULL DEFAULT 0,
		sunday     INTEGER NOT NULL DEFAULT 0,
		start_date TEXT NOT NULL,
		end_date   TEXT NOT NULL
	)`,

	// Calendar Dates (exceptions)
	`CREATE TABLE IF NOT EXISTS calendar_dates (
		service_id     TEXT NOT NULL,
		date           TEXT NOT NULL,
		exception_type INTEGER NOT NULL,
		PRIMARY KEY (service_id, date)
	)`,

	// Trips
	`CREATE TABLE IF NOT EXISTS trips (
		trip_id       TEXT PRIMARY KEY,
		route_id      TEXT NOT NULL REFERENCES routes(route_id),
		service_id    TEXT NOT NULL,
		trip_headsign TEXT,
		direction_id  INTEGER,
		block_id      TEXT,
		shape_id      TEXT
	)`,

	// Stop Times
	`CREATE TABLE IF NOT EXISTS stop_times (
		trip_id        TEXT NOT NULL REFERENCES trips(trip_id),
		arrival_time   TEXT NOT NULL,
		departure_time TEXT NOT NULL,
		stop_id        TEXT NOT NULL REFERENCES stops(stop_id),
		stop_sequence  INTEGER NOT NULL,
		pickup_type    INTEGER DEFAULT 0,
		drop_off_type  INTEGER DEFAULT 0,
		timepoint      INTEGER DEFAULT 0,
		PRIMARY KEY (trip_id, stop_sequence)
	)`,

	// Shapes (route geometry)
	`CREATE TABLE IF NOT EXISTS shapes (
		shape_id            TEXT NOT NULL,
		shape_pt_lat        REAL NOT NULL,
		shape_pt_lon        REAL NOT NULL,
		shape_pt_sequence   INTEGER NOT NULL,
		shape_dist_traveled REAL,
		PRIMARY KEY (shape_id, shape_pt_sequence)
	)`,

	// R-Tree spatial index on stops for nearest-stop queries
	`CREATE VIRTUAL TABLE IF NOT EXISTS stops_rtree USING rtree(
		id,
		min_lat, max_lat,
		min_lon, max_lon
	)`,

	// Feed metadata (last_modified, etag, imported_at, etc.)
	`CREATE TABLE IF NOT EXISTS feed_metadata (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,

	// Indexes for common query patterns
	`CREATE INDEX IF NOT EXISTS idx_stop_times_stop ON stop_times(stop_id)`,
	`CREATE INDEX IF NOT EXISTS idx_stop_times_trip ON stop_times(trip_id)`,
	`CREATE INDEX IF NOT EXISTS idx_stop_times_departure ON stop_times(stop_id, departure_time)`,
	`CREATE INDEX IF NOT EXISTS idx_trips_route ON trips(route_id)`,
	`CREATE INDEX IF NOT EXISTS idx_trips_service ON trips(service_id)`,
	`CREATE INDEX IF NOT EXISTS idx_trips_route_direction ON trips(route_id, direction_id)`,
	`CREATE INDEX IF NOT EXISTS idx_calendar_dates_date ON calendar_dates(date)`,

	// Users (auth)
	`CREATE TABLE IF NOT EXISTS users (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		username        TEXT UNIQUE NOT NULL,
		passphrase_hash TEXT NOT NULL,
		created_at      TEXT NOT NULL DEFAULT (datetime('now'))
	)`,

	// Device sessions (per-user device tracking)
	`CREATE TABLE IF NOT EXISTS device_sessions (
		user_id   INTEGER NOT NULL REFERENCES users(id),
		device_id TEXT NOT NULL,
		last_seen TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (user_id, device_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_device_sessions_user ON device_sessions(user_id, last_seen)`,
}
