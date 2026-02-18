package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gobus/internal/storage"
)

func withMiddleware(h http.Handler, logger *slog.Logger, cookieSecret []byte, db *storage.DB, ready <-chan struct{}) http.Handler {
	return securityHeaders(requestLogger(waitForData(requireAuth(h, cookieSecret, db), ready), logger))
}

// waitForData shows a loading page while GTFS data is being downloaded.
// Static assets and PWA files pass through so the loading page looks right.
func waitForData(next http.Handler, ready <-chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-ready:
			// Data is ready — pass through
			next.ServeHTTP(w, r)
			return
		default:
		}

		// Allow static assets, PWA files, and auth pages through while loading
		p := r.URL.Path
		if strings.HasPrefix(p, "/static/") || p == "/sw.js" ||
			p == "/manifest.json" || p == "/offline" ||
			p == "/login" || p == "/register" {
			next.ServeHTTP(w, r)
			return
		}

		// Show loading page
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(loadingPage))
	})
}

const loadingPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Loading — GoBus</title>
<meta http-equiv="refresh" content="5">
<meta name="theme-color" content="#1a1a2e">
<style>
  body {
    background: #1a1a2e;
    color: #e8e8e8;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    margin: 0;
  }
  .loading {
    text-align: center;
    padding: 2rem;
  }
  h1 { color: #fff; margin-bottom: 1rem; }
  p { color: #b0b0b0; font-size: 1.125rem; }
  .spinner {
    width: 40px; height: 40px;
    margin: 1.5rem auto;
    border: 4px solid #2a2a4a;
    border-top-color: #4cc9f0;
    border-radius: 50%;
    animation: spin 1s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
</head>
<body>
<div class="loading" role="status" aria-live="polite">
  <h1>GoBus</h1>
  <div class="spinner" aria-hidden="true"></div>
  <p>Please wait, downloading route data...</p>
  <p>This page will refresh automatically.</p>
</div>
</body>
</html>`

// requireAuth redirects unauthenticated requests to /login.
// Public paths are whitelisted and pass through without auth.
// On authenticated requests, updates the device session last_seen time.
func requireAuth(next http.Handler, secret []byte, db *storage.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path

		// Public paths — no auth required
		if p == "/login" || p == "/register" || p == "/offline" ||
			p == "/sw.js" || p == "/manifest.json" ||
			strings.HasPrefix(p, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		// Check session cookie
		cookie, err := r.Cookie("gobus_session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		userID := parseCookie(cookie.Value, secret)
		if userID == 0 {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Update device session last_seen (best-effort, don't block on error)
		if deviceCookie, err := r.Cookie("gobus_device"); err == nil && deviceCookie.Value != "" {
			db.UpsertDeviceSession(r.Context(), int64(userID), deviceCookie.Value)
		}

		next.ServeHTTP(w, r)
	})
}

// parseCookie verifies a "userID.expiry.hmac" cookie value.
// Returns userID on success, 0 on failure.
func parseCookie(value string, secret []byte) int64 {
	parts := strings.SplitN(value, ".", 3)
	if len(parts) != 3 {
		return 0
	}
	payload := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return 0
	}
	expiry, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return 0
	}
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || userID <= 0 {
		return 0
	}
	return userID
}

func requestLogger(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip logging for SSE connections (they're long-lived)
		if r.Header.Get("Accept") == "text/event-stream" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start).Round(time.Microsecond),
		)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// staticCacheHandler sets long cache headers on versioned static assets (?v=...).
// Unversioned requests get no-cache to ensure fresh content.
func staticCacheHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush exposes the underlying Flusher for SSE support.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
