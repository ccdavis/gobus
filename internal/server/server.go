package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
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
	mux        *http.ServeMux
	cfg        *config.Config
	logger     *slog.Logger
	db         *storage.DB
	ready      chan struct{} // closed when GTFS data is available
	httpServer *http.Server
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

	s := &Server{mux: mux, cfg: cfg, logger: logger, db: db, ready: ready}

	// Static files — served from embedded FS, versioned URLs get immutable caching
	staticFS, _ := fs.Sub(web.StaticFiles, "static")
	fileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticCacheHandler(fileServer)))

	// Pages
	mux.HandleFunc("GET /", h.Home)
	mux.HandleFunc("GET /nearby", h.Nearby)
	mux.HandleFunc("GET /search", h.Search)
	mux.HandleFunc("GET /routes", h.RouteList)
	mux.HandleFunc("GET /routes/{id}", h.RouteDetail)
	mux.HandleFunc("GET /stops/{id}", h.StopDetail)
	mux.HandleFunc("GET /stops/{stopID}/route/{routeID}", h.LaterArrivals)

	// API
	mux.HandleFunc("GET /api/location-label", h.LocationLabel)
	mux.HandleFunc("GET /api/saved", h.SavedList)
	mux.HandleFunc("POST /api/saved", h.SavedAdd)
	mux.HandleFunc("DELETE /api/saved/{stopID}", h.SavedRemove)
	mux.HandleFunc("POST /api/settings/unit", h.UnitSet)

	// SSE
	mux.HandleFunc("GET /sse/departures/{id}", h.SSEDepartures)

	// PWA
	mux.HandleFunc("GET /manifest.json", h.Manifest)
	mux.HandleFunc("GET /sw.js", h.ServiceWorker)
	mux.HandleFunc("GET /offline", h.Offline)

	s.httpServer = &http.Server{Handler: withMiddleware(s.mux, s.logger, s.ready)}

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

// Listen binds a TCP listener on host:port. A port of 0 lets the OS choose a
// free port; the actual port is returned. Binding to 127.0.0.1 keeps the
// server local to the device; set host to 0.0.0.0 to expose it on the LAN.
func Listen(host string, port int) (net.Listener, int, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, 0, fmt.Errorf("listen on %s:%d: %w", host, port, err)
	}
	return ln, ln.Addr().(*net.TCPAddr).Port, nil
}

// Serve serves HTTP on the given listener and blocks until the server is shut
// down. A clean shutdown via Shutdown returns nil rather than ErrServerClosed.
func (s *Server) Serve(ln net.Listener) error {
	s.logger.Info("server starting", "addr", ln.Addr().String())
	if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server, draining in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// ListenAndServe binds on the configured host/port and serves until shut down.
// Used by the desktop/standalone build; the mobile build calls Listen + Serve
// directly so it can capture the OS-assigned port.
func (s *Server) ListenAndServe() error {
	ln, port, err := Listen(s.cfg.Host, s.cfg.Port)
	if err != nil {
		return err
	}
	s.logger.Info("listening", "host", s.cfg.Host, "port", port)
	return s.Serve(ln)
}
