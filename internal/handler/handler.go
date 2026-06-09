package handler

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"

	"gobus/internal/config"
	"gobus/internal/geocode"
	"gobus/internal/nextrip"
	"gobus/internal/realtime"
	"gobus/internal/storage"
	"gobus/internal/templates"
	"gobus/web"
)

// cachedLocation stores the device's last reverse-geocoded location.
type cachedLocation struct {
	Lat     float64
	Lon     float64
	Address string
}

// Handler holds shared dependencies for all HTTP handlers.
type Handler struct {
	db            *storage.DB
	nt            *nextrip.Client
	rt            *realtime.Store
	geo           *geocode.Client
	cfg           *config.Config
	logger        *slog.Logger
	version       string   // content hash of static assets, for cache busting
	locationCache sync.Map // locationCacheKey → *cachedLocation (single user per device)
}

// New creates a Handler.
func New(db *storage.DB, nt *nextrip.Client, rt *realtime.Store, geo *geocode.Client, cfg *config.Config, logger *slog.Logger) *Handler {
	v := computeAssetVersion(web.StaticFiles)
	logger.Info("asset version computed", "version", v)

	return &Handler{db: db, nt: nt, rt: rt, geo: geo, cfg: cfg, logger: logger, version: v}
}

// computeAssetVersion hashes all CSS and JS files in the embedded static FS
// to produce a short version string. Changes to any file produce a new version.
func computeAssetVersion(staticFS fs.FS) string {
	h := md5.New()
	var paths []string
	fs.WalkDir(staticFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
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
		f, err := staticFS.Open(p)
		if err != nil {
			continue
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			continue
		}
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

