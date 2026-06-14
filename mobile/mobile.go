// Package mobile is the gomobile-bind entry point for the native iPhone app.
//
// It wraps the same Go core the desktop build uses (server + storage + nextrip
// + realtime) behind a tiny, gomobile-friendly API: Start brings up the local
// HTTP server bound to 127.0.0.1 and returns the chosen port; Stop shuts it
// down. The Swift shell calls Start on launch, then points a WKWebView at
// http://127.0.0.1:<port>/.
//
// Build (on macOS, with gomobile + Xcode — see NATIVE_APP_PLAN.md Phase 2):
//
//	gomobile bind -target=ios -o build/GobusKit.xcframework ./mobile
//
// gomobile prefixes the exported names with the package, so from Swift (after
// `import GobusKit`) these are MobileStart(...) and MobileStop(). The framework
// module is named GobusKit to avoid colliding with the Gobus app target.
package mobile

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"gobus/internal/config"
	"gobus/internal/gtfs"
	"gobus/internal/nextrip"
	"gobus/internal/realtime"
	"gobus/internal/server"
	"gobus/internal/storage"
)

const alertsURL = "https://svc.metrotransit.org/mtgtfs/alerts.pb"

// Guards the single running instance.
var (
	mu     sync.Mutex
	srv    *server.Server
	db     *storage.DB
	cancel context.CancelFunc
)

// Start opens the SQLite database at dbPath, begins serving GoBus on
// 127.0.0.1, and returns the actual TCP port. Pass port 0 to let the OS pick a
// free port (recommended on-device). dataDir is a writable directory for GTFS
// downloads/caching.
//
// The app should ship a prebuilt database (copied to a writable location on
// first launch); if the database has no schedule data, Start kicks off a GTFS
// download and import in the background and the UI shows a loading state until
// it completes. Start returns as soon as the listener is bound — serving runs
// in the background. Call Stop before calling Start again.
func Start(dbPath, dataDir string, port int) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	if srv != nil {
		return 0, fmt.Errorf("gobus: already started")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := config.Load()
	cfg.Host = "127.0.0.1"
	cfg.Port = port
	cfg.DBPath = dbPath
	cfg.GTFSDir = dataDir

	database, err := storage.Open(dbPath, logger)
	if err != nil {
		return 0, fmt.Errorf("gobus: open database: %w", err)
	}

	ctx, cancelFn := context.WithCancel(context.Background())

	nt := nextrip.NewClient(cfg.NexTripBaseURL, logger)
	rtStore := realtime.NewStore()
	go realtime.NewFetcher(alertsURL, rtStore, logger).Start(ctx)

	s := server.New(cfg, database, nt, rtStore, logger)

	// Ensure schedule data exists (no-op when the bundled DB already has it),
	// then keep it fresh in the background.
	scheduler := gtfs.NewScheduler(gtfs.NewDownloader(cfg.GTFSURL, cfg.GTFSDir, logger), database, logger)
	go func() {
		if err := scheduler.EnsureData(ctx); err != nil {
			logger.Error("ensure GTFS data", "error", err)
		}
		s.SetReady()
		scheduler.StartBackground(ctx)
	}()

	ln, actualPort, err := server.Listen(cfg.Host, port)
	if err != nil {
		cancelFn()
		database.Close()
		return 0, err
	}
	go func() {
		if err := s.Serve(ln); err != nil {
			logger.Error("gobus server stopped", "error", err)
		}
	}()

	srv, db, cancel = s, database, cancelFn
	return actualPort, nil
}

// Stop gracefully shuts down the server, stops background fetchers, and closes
// the database. Safe to call when not started.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if srv == nil {
		return
	}
	cancel()

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Default().Error("gobus shutdown", "error", err)
	}
	db.Close()

	srv, db, cancel = nil, nil, nil
}
