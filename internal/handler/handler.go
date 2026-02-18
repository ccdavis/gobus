package handler

import (
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"gobus/internal/config"
	"gobus/internal/geocode"
	"gobus/internal/nextrip"
	"gobus/internal/realtime"
	"gobus/internal/storage"
	"gobus/internal/templates"
)

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	db           *storage.DB
	nt           *nextrip.Client
	rt           *realtime.Store
	geo          *geocode.Client
	cfg          *config.Config
	logger       *slog.Logger
	version      string // content hash of static assets, for cache busting
	cookieSecret []byte // HMAC key for signing session cookies
}

// New creates a Handler.
func New(db *storage.DB, nt *nextrip.Client, rt *realtime.Store, geo *geocode.Client, cfg *config.Config, logger *slog.Logger) *Handler {
	v := computeAssetVersion("web/static")
	logger.Info("asset version computed", "version", v)

	// Derive cookie secret from config or generate a random one
	secret := []byte(cfg.CookieSecret)
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			logger.Error("failed to generate cookie secret", "error", err)
			os.Exit(1)
		}
		logger.Warn("no GOBUS_COOKIE_SECRET set — generated random secret (sessions won't survive restart)")
	}

	return &Handler{db: db, nt: nt, rt: rt, geo: geo, cfg: cfg, logger: logger, version: v, cookieSecret: secret}
}

// computeAssetVersion hashes all CSS and JS files in the static directory
// to produce a short version string. Changes to any file produce a new version.
func computeAssetVersion(staticDir string) string {
	h := md5.New()
	var paths []string
	filepath.Walk(staticDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		base := filepath.Base(path)
		if (ext == ".css" || ext == ".js") && base != "sw.js" {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths) // deterministic order
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		io.Copy(h, f)
		f.Close()
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:8]
}

// page creates a templates.Page with the asset version pre-filled.
func (h *Handler) page(title, currentPath string) templates.Page {
	return templates.Page{
		Title:        title,
		CurrentPath:  currentPath,
		AssetVersion: h.version,
	}
}
