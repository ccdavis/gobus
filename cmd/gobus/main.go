package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"gobus/internal/config"
	"gobus/internal/gtfs"
	"gobus/internal/nextrip"
	"gobus/internal/realtime"
	"gobus/internal/server"
	"gobus/internal/storage"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg := config.Load()

	// CLI flags
	importOnly := flag.Bool("import-gtfs", false, "Download and import GTFS data, then exit")
	flag.IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	flag.BoolVar(&cfg.TestMode, "test-mode", cfg.TestMode, "Enable test mode (fixture data, mock APIs)")
	flag.StringVar(&cfg.GTFSDir, "gtfs-dir", cfg.GTFSDir, "Directory for GTFS data files")
	flag.Parse()
	cfg.ImportGTFS = *importOnly

	// Context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Open database
	db, err := storage.Open(cfg.DBPath, logger)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Set up GTFS scheduler
	downloader := gtfs.NewDownloader(cfg.GTFSURL, cfg.GTFSDir, logger)
	scheduler := gtfs.NewScheduler(downloader, db, logger)

	// Handle --import-gtfs flag
	if cfg.ImportGTFS {
		logger.Info("force importing GTFS data")
		if err := scheduler.EnsureData(ctx); err != nil {
			logger.Error("GTFS import failed", "error", err)
			os.Exit(1)
		}
		logger.Info("GTFS import complete")
		return
	}

	// Ensure GTFS data exists (download on first run)
	if err := scheduler.EnsureData(ctx); err != nil {
		logger.Error("failed to ensure GTFS data", "error", err)
		// Continue anyway — the app can still serve route explorer from cached data
		// or show a helpful error message
	}

	// Start background GTFS update scheduler
	go scheduler.StartBackground(ctx)

	// Check for updates on first access today
	go func() {
		if err := scheduler.CheckAndUpdate(ctx); err != nil {
			logger.Error("daily GTFS check failed", "error", err)
		}
	}()

	// Create NexTrip API client
	nt := nextrip.NewClient(cfg.NexTripBaseURL, logger)

	// Start GTFS-RT realtime alerts fetcher
	rtStore := realtime.NewStore()
	alertsFetcher := realtime.NewFetcher(
		"https://svc.metrotransit.org/mtgtfs/alerts.pb",
		rtStore, logger,
	)
	go alertsFetcher.Start(ctx)

	// Start HTTP server
	srv := server.New(cfg, db, nt, rtStore, logger)

	// Graceful shutdown on SIGINT/SIGTERM
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down")
		cancel()
		os.Exit(0)
	}()

	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
