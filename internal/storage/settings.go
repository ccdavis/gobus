package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// SavedLocation is a user-saved stop (single-user, local-first).
type SavedLocation struct {
	StopID string  `json:"stopID"`
	Name   string  `json:"name"`
	Label  string  `json:"label"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
}

// GetSetting returns the value for a key. The bool is false if the key is unset.
func (db *DB) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get setting %q: %w", key, err)
	}
	return v, true, nil
}

// SetSetting stores (or overwrites) a key/value setting.
func (db *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}

// ListSavedLocations returns all saved locations, newest first.
func (db *DB) ListSavedLocations(ctx context.Context) ([]SavedLocation, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT stop_id, name, label, lat, lon FROM saved_locations ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list saved locations: %w", err)
	}
	defer rows.Close()

	var locs []SavedLocation
	for rows.Next() {
		var l SavedLocation
		if err := rows.Scan(&l.StopID, &l.Name, &l.Label, &l.Lat, &l.Lon); err != nil {
			return nil, fmt.Errorf("scan saved location: %w", err)
		}
		locs = append(locs, l)
	}
	return locs, rows.Err()
}

// AddSavedLocation inserts a saved location, updating the label/coords if the
// stop is already saved.
func (db *DB) AddSavedLocation(ctx context.Context, l SavedLocation) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO saved_locations (stop_id, name, label, lat, lon)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(stop_id) DO UPDATE SET
		   name = excluded.name, label = excluded.label,
		   lat = excluded.lat, lon = excluded.lon`,
		l.StopID, l.Name, l.Label, l.Lat, l.Lon)
	if err != nil {
		return fmt.Errorf("add saved location: %w", err)
	}
	return nil
}

// RemoveSavedLocation deletes a saved location by stop ID (no-op if absent).
func (db *DB) RemoveSavedLocation(ctx context.Context, stopID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM saved_locations WHERE stop_id = ?`, stopID)
	if err != nil {
		return fmt.Errorf("remove saved location: %w", err)
	}
	return nil
}

// IsSavedLocation reports whether a stop is saved.
func (db *DB) IsSavedLocation(ctx context.Context, stopID string) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx,
		`SELECT 1 FROM saved_locations WHERE stop_id = ?`, stopID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("is saved location: %w", err)
	}
	return true, nil
}
