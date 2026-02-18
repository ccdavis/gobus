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

func withMiddleware(h http.Handler, logger *slog.Logger, cookieSecret []byte, db *storage.DB) http.Handler {
	return securityHeaders(requestLogger(requireAuth(h, cookieSecret, db), logger))
}

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
