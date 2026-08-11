package gtfs

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gobus/internal/storage"
)

func testScheduler(t *testing.T, feedURL string) (*Scheduler, *storage.DB) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"), logger)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewScheduler(NewDownloader(feedURL, dir, logger), db, logger), db
}

// TestCheckAndUpdate_FailureRetriesSameDay pins the fix for marking a day as
// checked before the check succeeded: a failed check must not consume the
// day's single check slot.
func TestCheckAndUpdate_FailureRetriesSameDay(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	s, _ := testScheduler(t, srv.URL)
	ctx := context.Background()

	// First call: HEAD succeeds at transport level but the subsequent GET
	// (update) fails on status 500 — the day must not be marked checked.
	if err := s.CheckAndUpdate(ctx); err == nil {
		t.Fatal("CheckAndUpdate should fail against a 500 server")
	}
	first := calls.Load()
	if first == 0 {
		t.Fatal("no request made")
	}

	// Second call the same day must try again (not be swallowed by the
	// once-per-day gate).
	if err := s.CheckAndUpdate(ctx); err == nil {
		t.Fatal("second CheckAndUpdate should also fail")
	}
	if calls.Load() <= first {
		t.Errorf("failed check consumed the daily slot: no retry request was made")
	}
}

// TestCheckAndUpdate_NotModifiedMarksDayChecked verifies the happy no-op path
// still only checks once per day.
func TestCheckAndUpdate_NotModifiedMarksDayChecked(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotModified)
	}))
	t.Cleanup(srv.Close)

	s, _ := testScheduler(t, srv.URL)
	ctx := context.Background()

	if err := s.CheckAndUpdate(ctx); err != nil {
		t.Fatalf("CheckAndUpdate: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 request, got %d", calls.Load())
	}
	if err := s.CheckAndUpdate(ctx); err != nil {
		t.Fatalf("second CheckAndUpdate: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("successful check should gate further checks that day; got %d requests", calls.Load())
	}
}

// TestRefreshIfStale skips the network entirely when data is fresh and
// performs a conditional check when it isn't.
func TestRefreshIfStale(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotModified)
	}))
	t.Cleanup(srv.Close)

	s, db := testScheduler(t, srv.URL)
	ctx := context.Background()

	// Fresh import: no request.
	if err := db.SetMetadata(ctx, "imported_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshIfStale(ctx); err != nil {
		t.Fatalf("RefreshIfStale(fresh): %v", err)
	}
	if calls.Load() != 0 {
		t.Errorf("fresh data triggered %d network requests, want 0", calls.Load())
	}

	// Stale import: conditional check runs.
	if err := db.SetMetadata(ctx, "imported_at", time.Now().Add(-48*time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshIfStale(ctx); err != nil {
		t.Fatalf("RefreshIfStale(stale): %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("stale data triggered %d network requests, want 1", calls.Load())
	}
}
