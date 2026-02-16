package storage

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a SQLite database connection with GTFS-specific operations.
type DB struct {
	*sql.DB
	logger *slog.Logger
}

// Open creates or opens a SQLite database at the given path and applies migrations.
func Open(path string, logger *slog.Logger) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on", path)
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Verify connection
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := &DB{DB: sqlDB, logger: logger}

	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	logger.Info("database opened", "path", path)
	return db, nil
}
