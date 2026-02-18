package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"gobus/internal/config"
	"gobus/internal/geocode"
	"gobus/internal/handler"
	"gobus/internal/nextrip"
	"gobus/internal/realtime"
	"gobus/internal/storage"
	"gobus/web"
)

// Server is the HTTP server for GoBus.
type Server struct {
	mux          *http.ServeMux
	cfg          *config.Config
	logger       *slog.Logger
	db           *storage.DB
	cookieSecret []byte
	ready        chan struct{} // closed when GTFS data is available
}

// New creates a new Server with all routes registered.
func New(cfg *config.Config, db *storage.DB, nt *nextrip.Client, rt *realtime.Store, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	geo := geocode.New("GoBus/1.0 (transit PWA)")
	h := handler.New(db, nt, rt, geo, cfg, logger)

	ready := make(chan struct{})
	// If data already exists, mark ready immediately
	if db.HasData(context.Background()) {
		close(ready)
	}

	s := &Server{mux: mux, cfg: cfg, logger: logger, db: db, cookieSecret: h.CookieSecret(), ready: ready}

	// Static files — served from embedded FS, versioned URLs get immutable caching
	staticFS, _ := fs.Sub(web.StaticFiles, "static")
	fileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticCacheHandler(fileServer)))

	// Auth
	mux.HandleFunc("GET /login", h.Login)
	mux.HandleFunc("POST /login", h.Login)
	mux.HandleFunc("GET /register", h.Register)
	mux.HandleFunc("POST /register", h.Register)
	mux.HandleFunc("POST /logout", h.Logout)

	// Pages
	mux.HandleFunc("GET /", h.Home)
	mux.HandleFunc("GET /nearby", h.Nearby)
	mux.HandleFunc("GET /search", h.Search)
	mux.HandleFunc("GET /routes", h.RouteList)
	mux.HandleFunc("GET /routes/{id}", h.RouteDetail)
	mux.HandleFunc("GET /stops/{id}", h.StopDetail)
	mux.HandleFunc("GET /stops/{stopID}/route/{routeID}", h.LaterArrivals)

	// SSE
	mux.HandleFunc("GET /sse/departures/{id}", h.SSEDepartures)

	// PWA
	mux.HandleFunc("GET /manifest.json", h.Manifest)
	mux.HandleFunc("GET /sw.js", h.ServiceWorker)
	mux.HandleFunc("GET /offline", h.Offline)

	return s
}

// SetReady signals that GTFS data is available and the app can serve requests.
func (s *Server) SetReady() {
	select {
	case <-s.ready:
		// already closed
	default:
		close(s.ready)
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.cfg.Port)
	s.logger.Info("server starting", "addr", addr)
	return http.ListenAndServe(addr, withMiddleware(s.mux, s.logger, s.cookieSecret, s.db, s.ready))
}
