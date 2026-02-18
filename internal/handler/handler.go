package handler

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gobus/internal/config"
	"gobus/internal/geocode"
	"gobus/internal/nextrip"
	"gobus/internal/realtime"
	"gobus/internal/storage"
	"gobus/internal/templates"
	"gobus/web"
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
	v := computeAssetVersion(web.StaticFiles)
	logger.Info("asset version computed", "version", v)

	// Derive cookie secret: env var > file on disk > generate and save
	secret := loadOrCreateSecret(cfg, logger)

	return &Handler{db: db, nt: nt, rt: rt, geo: geo, cfg: cfg, logger: logger, version: v, cookieSecret: secret}
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

// loadOrCreateSecret resolves the cookie secret with this priority:
//  1. GOBUS_COOKIE_SECRET env var (for Fly.io / production)
//  2. .cookie_secret file next to the database (auto-persisted)
//  3. Generate random secret, write it to the file for next time
func loadOrCreateSecret(cfg *config.Config, logger *slog.Logger) []byte {
	// 1. Explicit env var takes priority
	if cfg.CookieSecret != "" {
		return []byte(cfg.CookieSecret)
	}

	// 2. Try reading from file next to the database
	secretPath := filepath.Join(filepath.Dir(cfg.DBPath), ".cookie_secret")
	if data, err := os.ReadFile(secretPath); err == nil {
		s := strings.TrimSpace(string(data))
		if decoded, err := hex.DecodeString(s); err == nil && len(decoded) >= 16 {
			logger.Info("cookie secret loaded from file", "path", secretPath)
			return decoded
		}
	}

	// 3. Generate and save
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		logger.Error("failed to generate cookie secret", "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(secretPath), 0700); err == nil {
		if err := os.WriteFile(secretPath, []byte(hex.EncodeToString(secret)+"\n"), 0600); err == nil {
			logger.Info("cookie secret generated and saved", "path", secretPath)
		} else {
			logger.Warn("could not save cookie secret to file — sessions won't survive restart", "error", err)
		}
	}
	return secret
}
