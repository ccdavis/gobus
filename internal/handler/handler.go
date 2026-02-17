package handler

import (
	"log/slog"

	"gobus/internal/config"
	"gobus/internal/geocode"
	"gobus/internal/nextrip"
	"gobus/internal/realtime"
	"gobus/internal/storage"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	db     *storage.DB
	nt     *nextrip.Client
	rt     *realtime.Store
	geo    *geocode.Client
	cfg    *config.Config
	logger *slog.Logger
}

// New creates a Handler.
func New(db *storage.DB, nt *nextrip.Client, rt *realtime.Store, geo *geocode.Client, cfg *config.Config, logger *slog.Logger) *Handler {
	return &Handler{db: db, nt: nt, rt: rt, geo: geo, cfg: cfg, logger: logger}
}
