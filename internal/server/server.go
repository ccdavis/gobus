package server

import (
	"fmt"
	"log/slog"
	"net/http"

	"gobus/internal/config"
	"gobus/internal/handler"
	"gobus/internal/nextrip"
	"gobus/internal/realtime"
	"gobus/internal/storage"
)

// Server is the HTTP server for GoBus.
type Server struct {
	mux    *http.ServeMux
	cfg    *config.Config
	logger *slog.Logger
}

// New creates a new Server with all routes registered.
func New(cfg *config.Config, db *storage.DB, nt *nextrip.Client, rt *realtime.Store, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	s := &Server{mux: mux, cfg: cfg, logger: logger}

	h := handler.New(db, nt, rt, cfg, logger)

	// Static files
	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fs))

	// Pages
	mux.HandleFunc("GET /", h.Home)
	mux.HandleFunc("GET /nearby", h.Nearby)
	mux.HandleFunc("GET /routes", h.RouteList)
	mux.HandleFunc("GET /routes/{id}", h.RouteDetail)
	mux.HandleFunc("GET /stops/{id}", h.StopDetail)

	// SSE
	mux.HandleFunc("GET /sse/departures/{id}", h.SSEDepartures)

	// PWA
	mux.HandleFunc("GET /manifest.json", h.Manifest)
	mux.HandleFunc("GET /sw.js", h.ServiceWorker)
	mux.HandleFunc("GET /offline", h.Offline)

	return s
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	s.logger.Info("server starting", "addr", addr)
	return http.ListenAndServe(addr, withMiddleware(s.mux, s.logger))
}
