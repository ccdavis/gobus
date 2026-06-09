package storage

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := Open(filepath.Join(t.TempDir(), "test.db"), logger)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSettingsRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Unset key returns found=false.
	if _, ok, err := db.GetSetting(ctx, "distance_unit"); err != nil || ok {
		t.Fatalf("GetSetting(unset) = ok=%v err=%v, want ok=false err=nil", ok, err)
	}

	if err := db.SetSetting(ctx, "distance_unit", "imperial"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	v, ok, err := db.GetSetting(ctx, "distance_unit")
	if err != nil || !ok || v != "imperial" {
		t.Fatalf("GetSetting = (%q, %v, %v), want (imperial, true, nil)", v, ok, err)
	}

	// Overwrite.
	if err := db.SetSetting(ctx, "distance_unit", "metric"); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}
	if v, _, _ := db.GetSetting(ctx, "distance_unit"); v != "metric" {
		t.Fatalf("after overwrite = %q, want metric", v)
	}
}

func TestSavedLocationsCRUD(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	if locs, err := db.ListSavedLocations(ctx); err != nil || len(locs) != 0 {
		t.Fatalf("ListSavedLocations(empty) = (%d, %v), want (0, nil)", len(locs), err)
	}
	if saved, err := db.IsSavedLocation(ctx, "S1"); err != nil || saved {
		t.Fatalf("IsSavedLocation(absent) = (%v, %v), want (false, nil)", saved, err)
	}

	loc := SavedLocation{StopID: "S1", Name: "Main St", Label: "Home", Lat: 44.97, Lon: -93.26}
	if err := db.AddSavedLocation(ctx, loc); err != nil {
		t.Fatalf("AddSavedLocation: %v", err)
	}

	locs, err := db.ListSavedLocations(ctx)
	if err != nil || len(locs) != 1 || locs[0] != loc {
		t.Fatalf("ListSavedLocations = (%+v, %v), want one matching %+v", locs, err, loc)
	}
	if saved, _ := db.IsSavedLocation(ctx, "S1"); !saved {
		t.Fatal("IsSavedLocation(S1) = false, want true")
	}

	// Re-adding the same stop updates rather than duplicating.
	loc.Label = "Work"
	if err := db.AddSavedLocation(ctx, loc); err != nil {
		t.Fatalf("AddSavedLocation(update): %v", err)
	}
	locs, _ = db.ListSavedLocations(ctx)
	if len(locs) != 1 || locs[0].Label != "Work" {
		t.Fatalf("after re-add = %+v, want single entry labeled Work", locs)
	}

	if err := db.RemoveSavedLocation(ctx, "S1"); err != nil {
		t.Fatalf("RemoveSavedLocation: %v", err)
	}
	if locs, _ := db.ListSavedLocations(ctx); len(locs) != 0 {
		t.Fatalf("after remove = %+v, want empty", locs)
	}
}
