package gtfs

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"time"

	"gobus/internal/storage"
)

// Importer loads parsed GTFS data into SQLite.
type Importer struct {
	db     *storage.DB
	logger *slog.Logger
}

// NewImporter creates an Importer.
func NewImporter(db *storage.DB, logger *slog.Logger) *Importer {
	return &Importer{db: db, logger: logger}
}

// Import loads a parsed GTFS feed plus streams stop_times and shapes from the zip file.
// The entire operation runs in a single transaction for atomicity.
func (imp *Importer) Import(ctx context.Context, feed *Feed, zipPath string) error {
	start := time.Now()

	tx, err := imp.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear existing data
	if err := imp.clearTables(ctx, tx); err != nil {
		return err
	}

	// Import in-memory tables
	if err := imp.importAgencies(ctx, tx, feed.Agencies); err != nil {
		return err
	}
	if err := imp.importRoutes(ctx, tx, feed.Routes); err != nil {
		return err
	}
	if err := imp.importStops(ctx, tx, feed.Stops); err != nil {
		return err
	}
	if err := imp.importCalendar(ctx, tx, feed.Calendar); err != nil {
		return err
	}
	if err := imp.importCalendarDates(ctx, tx, feed.CalendarDates); err != nil {
		return err
	}
	if err := imp.importTrips(ctx, tx, feed.Trips); err != nil {
		return err
	}

	// Stream large tables directly from zip
	if err := imp.streamStopTimes(ctx, tx, zipPath); err != nil {
		return err
	}
	if err := imp.streamShapes(ctx, tx, zipPath); err != nil {
		return err
	}

	// Rebuild R-Tree spatial index
	if err := imp.db.RebuildRTree(ctx, tx); err != nil {
		return fmt.Errorf("rebuild rtree: %w", err)
	}

	// Store metadata
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO feed_metadata (key, value) VALUES ('imported_at', ?)`, now); err != nil {
		return fmt.Errorf("set imported_at: %w", err)
	}
	if feed.LastModified != "" {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO feed_metadata (key, value) VALUES ('last_modified', ?)`, feed.LastModified); err != nil {
			return fmt.Errorf("set last_modified: %w", err)
		}
	}
	if feed.ETag != "" {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO feed_metadata (key, value) VALUES ('etag', ?)`, feed.ETag); err != nil {
			return fmt.Errorf("set etag: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	imp.logger.Info("GTFS import complete",
		"duration", time.Since(start).Round(time.Millisecond),
		"routes", len(feed.Routes),
		"stops", len(feed.Stops),
		"trips", len(feed.Trips),
	)
	return nil
}

func (imp *Importer) clearTables(ctx context.Context, tx *sql.Tx) error {
	tables := []string{
		"stop_times", "shapes", "trips", "calendar_dates", "calendar",
		"stops", "routes", "agency", "stops_rtree", "feed_metadata",
	}
	for _, t := range tables {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", t)); err != nil {
			return fmt.Errorf("clear %s: %w", t, err)
		}
	}
	return nil
}

func (imp *Importer) importAgencies(ctx context.Context, tx *sql.Tx, agencies []Agency) error {
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO agency (agency_id, agency_name, agency_url, agency_timezone) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare agency: %w", err)
	}
	defer stmt.Close()

	for _, a := range agencies {
		if _, err := stmt.ExecContext(ctx, a.AgencyID, a.AgencyName, a.AgencyURL, a.AgencyTimezone); err != nil {
			return fmt.Errorf("insert agency %s: %w", a.AgencyID, err)
		}
	}
	imp.logger.Info("imported agencies", "count", len(agencies))
	return nil
}

func (imp *Importer) importRoutes(ctx context.Context, tx *sql.Tx, routes []Route) error {
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO routes (route_id, agency_id, route_short_name, route_long_name,
		 route_type, route_color, route_text_color, route_sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare routes: %w", err)
	}
	defer stmt.Close()

	for _, r := range routes {
		if _, err := stmt.ExecContext(ctx, r.RouteID, r.AgencyID, r.RouteShortName,
			r.RouteLongName, r.RouteType, r.RouteColor, r.RouteTextColor, r.RouteSortOrder); err != nil {
			return fmt.Errorf("insert route %s: %w", r.RouteID, err)
		}
	}
	imp.logger.Info("imported routes", "count", len(routes))
	return nil
}

func (imp *Importer) importStops(ctx context.Context, tx *sql.Tx, stops []Stop) error {
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO stops (stop_id, stop_code, stop_name, stop_desc, stop_lat, stop_lon,
		 zone_id, stop_url, location_type, parent_station, wheelchair_boarding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare stops: %w", err)
	}
	defer stmt.Close()

	for _, s := range stops {
		if _, err := stmt.ExecContext(ctx, s.StopID, s.StopCode, s.StopName, s.StopDesc,
			s.StopLat, s.StopLon, s.ZoneID, s.StopURL, s.LocationType,
			s.ParentStation, s.WheelchairBoarding); err != nil {
			return fmt.Errorf("insert stop %s: %w", s.StopID, err)
		}
	}
	imp.logger.Info("imported stops", "count", len(stops))
	return nil
}

func (imp *Importer) importCalendar(ctx context.Context, tx *sql.Tx, entries []CalendarEntry) error {
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO calendar (service_id, monday, tuesday, wednesday, thursday,
		 friday, saturday, sunday, start_date, end_date)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare calendar: %w", err)
	}
	defer stmt.Close()

	for _, c := range entries {
		if _, err := stmt.ExecContext(ctx, c.ServiceID, c.Monday, c.Tuesday, c.Wednesday,
			c.Thursday, c.Friday, c.Saturday, c.Sunday, c.StartDate, c.EndDate); err != nil {
			return fmt.Errorf("insert calendar %s: %w", c.ServiceID, err)
		}
	}
	imp.logger.Info("imported calendar entries", "count", len(entries))
	return nil
}

func (imp *Importer) importCalendarDates(ctx context.Context, tx *sql.Tx, dates []CalendarDate) error {
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO calendar_dates (service_id, date, exception_type) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare calendar_dates: %w", err)
	}
	defer stmt.Close()

	for _, d := range dates {
		if _, err := stmt.ExecContext(ctx, d.ServiceID, d.Date, d.ExceptionType); err != nil {
			return fmt.Errorf("insert calendar_date %s/%s: %w", d.ServiceID, d.Date, err)
		}
	}
	imp.logger.Info("imported calendar dates", "count", len(dates))
	return nil
}

func (imp *Importer) importTrips(ctx context.Context, tx *sql.Tx, trips []Trip) error {
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO trips (trip_id, route_id, service_id, trip_headsign,
		 direction_id, block_id, shape_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare trips: %w", err)
	}
	defer stmt.Close()

	for _, t := range trips {
		if _, err := stmt.ExecContext(ctx, t.TripID, t.RouteID, t.ServiceID,
			t.TripHeadsign, t.DirectionID, t.BlockID, t.ShapeID); err != nil {
			return fmt.Errorf("insert trip %s: %w", t.TripID, err)
		}
	}
	imp.logger.Info("imported trips", "count", len(trips))
	return nil
}

// streamStopTimes reads stop_times.txt directly from the zip in a streaming fashion.
func (imp *Importer) streamStopTimes(ctx context.Context, tx *sql.Tx, zipPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip for stop_times: %w", err)
	}
	defer r.Close()

	var stopTimesFile *zip.File
	for _, f := range r.File {
		if f.Name == "stop_times.txt" {
			stopTimesFile = f
			break
		}
	}
	if stopTimesFile == nil {
		return fmt.Errorf("stop_times.txt not found in zip")
	}

	streamer, err := OpenCSVStream[StopTime](stopTimesFile)
	if err != nil {
		return fmt.Errorf("open stop_times stream: %w", err)
	}
	defer streamer.Close()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO stop_times (trip_id, arrival_time, departure_time, stop_id,
		 stop_sequence, pickup_type, drop_off_type, timepoint)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare stop_times: %w", err)
	}
	defer stmt.Close()

	count := 0
	var st StopTime
	for {
		err := streamer.Next(&st)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read stop_time row %d: %w", count, err)
		}

		if _, err := stmt.ExecContext(ctx, st.TripID, st.ArrivalTime, st.DepartureTime,
			st.StopID, st.StopSequence, st.PickupType, st.DropOffType, st.Timepoint); err != nil {
			return fmt.Errorf("insert stop_time row %d: %w", count, err)
		}
		count++

		if count%500000 == 0 {
			imp.logger.Info("importing stop_times", "rows", count)
		}
	}

	imp.logger.Info("imported stop_times", "count", count)
	return nil
}

// streamShapes reads shapes.txt directly from the zip in a streaming fashion.
func (imp *Importer) streamShapes(ctx context.Context, tx *sql.Tx, zipPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip for shapes: %w", err)
	}
	defer r.Close()

	var shapesFile *zip.File
	for _, f := range r.File {
		if f.Name == "shapes.txt" {
			shapesFile = f
			break
		}
	}
	if shapesFile == nil {
		// shapes.txt is optional in GTFS
		imp.logger.Info("shapes.txt not found in zip, skipping")
		return nil
	}

	streamer, err := OpenCSVStream[ShapePoint](shapesFile)
	if err != nil {
		return fmt.Errorf("open shapes stream: %w", err)
	}
	defer streamer.Close()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO shapes (shape_id, shape_pt_lat, shape_pt_lon, shape_pt_sequence, shape_dist_traveled)
		 VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare shapes: %w", err)
	}
	defer stmt.Close()

	count := 0
	var sp ShapePoint
	for {
		err := streamer.Next(&sp)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read shape row %d: %w", count, err)
		}

		dist := sp.ShapeDistTraveled
		if dist == "" {
			dist = "0"
		}
		if _, err := stmt.ExecContext(ctx, sp.ShapeID, sp.ShapePtLat, sp.ShapePtLon,
			sp.ShapePtSequence, dist); err != nil {
			return fmt.Errorf("insert shape row %d: %w", count, err)
		}
		count++

		if count%500000 == 0 {
			imp.logger.Info("importing shapes", "rows", count)
		}
	}

	imp.logger.Info("imported shapes", "count", count)
	return nil
}
