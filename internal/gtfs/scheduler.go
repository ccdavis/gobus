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
	lastCheckDate string // YYYY-MM-DD of last check, prevents multiple checks per day
}

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

// CheckAndUpdate checks if the feed has been updated and imports it if so.
// Only checks once per calendar day.
func (s *Scheduler) CheckAndUpdate(ctx context.Context) error {
	s.mu.Lock()
	today := time.Now().In(chicagoTZ()).Format("2006-01-02")
	if s.lastCheckDate == today {
		s.mu.Unlock()
		return nil
	}
	s.lastCheckDate = today
	s.mu.Unlock()

	lastModified, _ := s.db.GetMetadata(ctx, "last_modified")
	etag, _ := s.db.GetMetadata(ctx, "etag")

	result, err := s.downloader.Check(ctx, lastModified, etag)
	if err != nil {
		return err
	}
	if !result.NeedsUpdate {
		return nil
	}

	return s.update(ctx)
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
