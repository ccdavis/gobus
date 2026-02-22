package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gobus/internal/templates"
)

const (
	cookieName     = "gobus_session"
	deviceCookie   = "gobus_device"
	cookieMaxAge   = 30 * 24 * 60 * 60 // 30 days in seconds
	timeGateMinSec = 3                  // minimum seconds between form load and submit
)

// --- Cookie signing / verification ---

// signCookie produces "userID.expiry.hmac" for a session cookie.
func (h *Handler) signCookie(userID int64) string {
	expiry := time.Now().Unix() + cookieMaxAge
	payload := fmt.Sprintf("%d.%d", userID, expiry)
	mac := hmac.New(sha256.New, h.cookieSecret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

// VerifyCookie checks a "userID.expiry.hmac" cookie value.
// Returns userID on success, 0 on failure.
// Exported so middleware can share the same implementation.
func VerifyCookie(value string, secret []byte) int64 {
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

// verifyCookie is a convenience method that calls the shared VerifyCookie.
func (h *Handler) verifyCookie(value string) int64 {
	return VerifyCookie(value, h.cookieSecret)
}

// setCookie sets the session cookie on the response.
func (h *Handler) setCookie(w http.ResponseWriter, userID int64) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    h.signCookie(userID),
		Path:     "/",
		MaxAge:   cookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearCookie removes the session cookie.
func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// --- Device cookie ---

// getOrCreateDeviceID reads the device cookie, or generates a new one and sets it.
func (h *Handler) getOrCreateDeviceID(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(deviceCookie); err == nil && c.Value != "" {
		return c.Value
	}
	id := generateDeviceID()
	http.SetCookie(w, &http.Cookie{
		Name:     deviceCookie,
		Value:    id,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60, // 1 year
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return id
}

func generateDeviceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// checkDeviceLimits verifies that adding this device won't exceed limits.
// Returns an error message if the limit is exceeded, or "" if OK.
// If the absolute cap is reached, the oldest device is evicted to make room.
func (h *Handler) checkDeviceLimits(r *http.Request, userID int64, deviceID string) string {
	ctx := r.Context()

	// Check temporal limit: too many distinct devices in the rolling window?
	if h.cfg.MaxDevicesRecent > 0 {
		// First check if this device is already known in the window
		alreadyKnown, err := h.db.IsDeviceRecent(ctx, userID, deviceID, h.cfg.DeviceWindowMin)
		if err != nil {
			h.logger.Error("device limit: check device recent", "error", err)
			return "Something went wrong. Please try again."
		}
		if !alreadyKnown {
			// This would be a new device — check if we're at the limit
			recent, err := h.db.CountRecentDevices(ctx, userID, h.cfg.DeviceWindowMin)
			if err != nil {
				h.logger.Error("device limit: count recent", "error", err)
				return "Something went wrong. Please try again."
			}
			if recent >= h.cfg.MaxDevicesRecent {
				return fmt.Sprintf("Too many devices. This account is active on %d devices right now. Please try again later.", recent)
			}
		}
	}

	// Enforce absolute cap: evict oldest if at limit
	if h.cfg.MaxDevicesTotal > 0 {
		total, err := h.db.CountDevicesForUser(ctx, userID)
		if err != nil {
			h.logger.Error("device limit: count total", "error", err)
			return "Something went wrong. Please try again."
		}
		for total >= h.cfg.MaxDevicesTotal {
			if err := h.db.EvictOldestDevice(ctx, userID); err != nil {
				h.logger.Error("device limit: evict", "error", err)
				break
			}
			total--
		}
	}

	return ""
}

// recordDevice upserts the device session for tracking.
func (h *Handler) recordDevice(r *http.Request, userID int64, deviceID string) {
	if err := h.db.UpsertDeviceSession(r.Context(), userID, deviceID); err != nil {
		h.logger.Error("recording device session", "error", err)
	}
}

// --- Time gate token ---

// timeGateToken creates a signed timestamp token for anti-bot time gating.
func (h *Handler) timeGateToken() string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, h.cookieSecret)
	mac.Write([]byte(ts))
	return ts + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifyTimeGate checks the time gate token. Returns true if valid and enough time has passed.
func (h *Handler) verifyTimeGate(token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	mac := hmac.New(sha256.New, h.cookieSecret)
	mac.Write([]byte(parts[0]))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return false
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix()-ts >= timeGateMinSec
}

// TestSignCookie creates a signed cookie for testing purposes.
// expiryOffset is seconds from now (positive = future, negative = expired).
func TestSignCookie(userID int64, expiryOffset int64, secret []byte) string {
	expiry := time.Now().Unix() + expiryOffset
	payload := fmt.Sprintf("%d.%d", userID, expiry)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

// --- Handlers ---

// Login handles GET (show form) and POST (verify credentials).
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.loginPost(w, r)
		return
	}
	h.renderLogin(w, r, "")
}

func (h *Handler) renderLogin(w http.ResponseWriter, r *http.Request, errMsg string) {
	data := templates.AuthData{
		Page:     h.page("Login", "/login"),
		IsLogin:  true,
		Error:    errMsg,
		Username: r.FormValue("username"),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if errMsg != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	if err := templates.AuthPage(data).Render(r.Context(), w); err != nil {
		h.logger.Error("rendering login page", "error", err)
	}
}

func (h *Handler) loginPost(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	passphrase := r.FormValue("passphrase")

	if username == "" || passphrase == "" {
		h.renderLogin(w, r, "Username and passphrase are required.")
		return
	}

	user, err := h.db.GetUserByUsername(r.Context(), username)
	if err == sql.ErrNoRows {
		h.renderLogin(w, r, "Invalid username or passphrase.")
		return
	}
	if err != nil {
		h.logger.Error("login: db lookup", "error", err)
		h.renderLogin(w, r, "Something went wrong. Please try again.")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PassphraseHash), []byte(passphrase)); err != nil {
		h.renderLogin(w, r, "Invalid username or passphrase.")
		return
	}

	// Device limiting
	deviceID := h.getOrCreateDeviceID(w, r)
	if msg := h.checkDeviceLimits(r, int64(user.ID), deviceID); msg != "" {
		h.renderLogin(w, r, msg)
		return
	}

	h.recordDevice(r, int64(user.ID), deviceID)
	h.setCookie(w, int64(user.ID))
	h.logger.Info("user logged in", "username", username, "device", deviceID[:8])
	http.Redirect(w, r, "/nearby", http.StatusSeeOther)
}

// Register handles GET (show form) and POST (create user).
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.registerPost(w, r)
		return
	}
	h.renderRegister(w, r, "")
}

func (h *Handler) renderRegister(w http.ResponseWriter, r *http.Request, errMsg string) {
	data := templates.AuthData{
		Page:     h.page("Register", "/register"),
		IsLogin:  false,
		Error:    errMsg,
		Username: r.FormValue("username"),
		TimeGate: h.timeGateToken(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if errMsg != "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}
	if err := templates.AuthPage(data).Render(r.Context(), w); err != nil {
		h.logger.Error("rendering register page", "error", err)
	}
}

func (h *Handler) registerPost(w http.ResponseWriter, r *http.Request) {
	// Honeypot check — if the hidden "website" field is filled, silently reject
	if r.FormValue("website") != "" {
		h.logger.Info("registration rejected: honeypot triggered")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Time gate check
	if !h.verifyTimeGate(r.FormValue("ts")) {
		h.renderRegister(w, r, "Please wait a moment before submitting.")
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	passphrase := r.FormValue("passphrase")

	if username == "" || passphrase == "" {
		h.renderRegister(w, r, "Username and passphrase are required.")
		return
	}

	if len(username) < 3 || len(username) > 30 {
		h.renderRegister(w, r, "Username must be 3-30 characters.")
		return
	}

	if len(passphrase) < 8 {
		h.renderRegister(w, r, "Passphrase must be at least 8 characters.")
		return
	}

	// Check registration cap
	if h.cfg.MaxUsers > 0 {
		count, err := h.db.CountUsers(r.Context())
		if err != nil {
			h.logger.Error("registration: count users", "error", err)
			h.renderRegister(w, r, "Something went wrong. Please try again.")
			return
		}
		if count >= h.cfg.MaxUsers {
			h.renderRegister(w, r, "Registration is currently closed.")
			return
		}
	}

	// Hash passphrase
	hash, err := bcrypt.GenerateFromPassword([]byte(passphrase), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("registration: bcrypt", "error", err)
		h.renderRegister(w, r, "Something went wrong. Please try again.")
		return
	}

	// Create user
	userID, err := h.db.CreateUser(r.Context(), username, string(hash))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			h.renderRegister(w, r, "That username is already taken.")
			return
		}
		h.logger.Error("registration: create user", "error", err)
		h.renderRegister(w, r, "Something went wrong. Please try again.")
		return
	}

	// Record device for new user (no limit check needed — first device)
	deviceID := h.getOrCreateDeviceID(w, r)
	h.recordDevice(r, userID, deviceID)

	h.setCookie(w, userID)
	h.logger.Info("user registered", "username", username, "id", userID, "device", deviceID[:8])
	http.Redirect(w, r, "/nearby", http.StatusSeeOther)
}

// Logout clears the session cookie.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.clearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// CookieSecret returns the handler's cookie secret for use by middleware.
func (h *Handler) CookieSecret() []byte {
	return h.cookieSecret
}
