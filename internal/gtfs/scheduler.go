package gtfs

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"gobus/internal/storage"
)

// Scheduler manages periodic GTFS feed updates.
type Scheduler struct {
	downloader *Downloader
	importer   *Importer
	db         *storage.DB
	logger     *slog.Logger

	mu            sync.Mutex
	lastCheckDate string // YYYY-MM-DD of last successful check, prevents multiple checks per day
	checking      bool   // a check/update is in flight
}

// staleAfter is how old imported schedule data may get before a foreground
// launch triggers a conditional refresh. The 3 AM background timer handles
// always-running desktop processes; iOS suspends the app, so refresh has to
// piggyback on app usage instead.
const staleAfter = 24 * time.Hour

// NewScheduler creates a Scheduler.
func NewScheduler(downloader *Downloader, db *storage.DB, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		downloader: downloader,
		importer:   NewImporter(db, logger),
		db:         db,
		logger:     logger,
	}
}

// EnsureData downloads and imports GTFS data if the database is empty.
// Called on startup.
func (s *Scheduler) EnsureData(ctx context.Context) error {
	if s.db.HasData(ctx) {
		s.logger.Info("GTFS data already present")
		return nil
	}
	s.logger.Info("no GTFS data found, performing initial import")
	return s.update(ctx)
}

// ForceUpdate unconditionally downloads and imports the feed, replacing any
// existing data. Used by the --import-gtfs CLI flag.
func (s *Scheduler) ForceUpdate(ctx context.Context) error {
	return s.update(ctx)
}

// CheckAndUpdate checks if the feed has been updated and imports it if so.
// Checks at most once per calendar day — but a day only counts as checked
// once the check (and any resulting import) succeeds, so a network failure
// is retried on the next call rather than silently skipped until tomorrow.
func (s *Scheduler) CheckAndUpdate(ctx context.Context) error {
	today := time.Now().In(chicagoTZ()).Format("2006-01-02")

	s.mu.Lock()
	if s.lastCheckDate == today || s.checking {
		s.mu.Unlock()
		return nil
	}
	s.checking = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.checking = false
		s.mu.Unlock()
	}()

	lastModified, _ := s.db.GetMetadata(ctx, "last_modified")
	etag, _ := s.db.GetMetadata(ctx, "etag")

	result, err := s.downloader.Check(ctx, lastModified, etag)
	if err != nil {
		return err
	}
	if result.NeedsUpdate {
		if err := s.update(ctx); err != nil {
			return err
		}
	}

	s.mu.Lock()
	s.lastCheckDate = today
	s.mu.Unlock()
	return nil
}

// RefreshIfStale triggers a conditional feed check when the imported data is
// older than staleAfter (or its age is unknown). Called on app launch and
// foreground so schedule data stays current on devices — like iPhones — where
// the process is suspended long before the 3 AM background timer fires.
// Safe to call often; it no-ops when data is fresh or a check already ran today.
func (s *Scheduler) RefreshIfStale(ctx context.Context) error {
	importedAt, _ := s.db.GetMetadata(ctx, "imported_at")
	if importedAt != "" {
		if t, err := time.Parse(time.RFC3339, importedAt); err == nil && time.Since(t) < staleAfter {
			return nil
		}
	}
	s.logger.Info("schedule data stale, checking for feed update", "imported_at", importedAt)
	return s.CheckAndUpdate(ctx)
}

// StartBackground starts the 3 AM daily check goroutine.
// It blocks until the context is cancelled.
func (s *Scheduler) StartBackground(ctx context.Context) {
	s.logger.Info("GTFS background scheduler started")

	for {
		next := next3AM()
		s.logger.Info("next GTFS check scheduled", "at", next.Format(time.RFC3339))

		timer := time.NewTimer(time.Until(next))
		select {
		case <-timer.C:
			if err := s.CheckAndUpdate(ctx); err != nil {
				s.logger.Error("background GTFS update failed", "error", err)
			}
		case <-ctx.Done():
			timer.Stop()
			s.logger.Info("GTFS background scheduler stopped")
			return
		}
	}
}

// update performs a full download-parse-import cycle.
func (s *Scheduler) update(ctx context.Context) error {
	zipPath, lastModified, etag, err := s.downloader.Download(ctx)
	if err != nil {
		return err
	}
	defer os.Remove(zipPath)

	feed, err := ParseZip(zipPath, s.logger)
	if err != nil {
		return err
	}
	feed.LastModified = lastModified
	feed.ETag = etag

	return s.importer.Import(ctx, feed, zipPath)
}

// next3AM returns the next 3:00 AM Central time.
func next3AM() time.Time {
	loc := chicagoTZ()
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, loc)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func chicagoTZ() *time.Location {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		// Fallback: use fixed offset for Central Time (-6h)
		loc = time.FixedZone("CST", -6*60*60)
	}
	return loc
}
