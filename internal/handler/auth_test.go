package handler

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

func newTestHandler() *Handler {
	return &Handler{
		cookieSecret: []byte("test-secret-32-bytes-long-xxxxx!"),
		logger:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
}

func TestSignCookie_Format(t *testing.T) {
	h := newTestHandler()
	signed := h.signCookie(42)

	parts := strings.SplitN(signed, ".", 3)
	if len(parts) != 3 {
		t.Fatalf("signCookie should produce 3 dot-separated parts, got %d: %q", len(parts), signed)
	}
	if parts[0] != "42" {
		t.Errorf("first part should be userID '42', got %q", parts[0])
	}
	if len(parts[2]) != 64 {
		t.Errorf("signature should be 64 hex chars, got %d: %q", len(parts[2]), parts[2])
	}
}

func TestSignVerifyCookie_RoundTrip(t *testing.T) {
	h := newTestHandler()

	tests := []int64{1, 42, 100, 999999}
	for _, userID := range tests {
		signed := h.signCookie(userID)
		got := h.verifyCookie(signed)
		if got != userID {
			t.Errorf("verifyCookie(signCookie(%d)) = %d, want %d", userID, got, userID)
		}
	}
}

func TestVerifyCookie_TamperedSignature(t *testing.T) {
	h := newTestHandler()
	signed := h.signCookie(42)

	tampered := signed[:len(signed)-1] + "x"
	if got := h.verifyCookie(tampered); got != 0 {
		t.Errorf("tampered signature should return 0, got %d", got)
	}
}

func TestVerifyCookie_TamperedUserID(t *testing.T) {
	h := newTestHandler()
	signed := h.signCookie(42)

	parts := strings.SplitN(signed, ".", 3)
	tampered := "99." + parts[1] + "." + parts[2]
	if got := h.verifyCookie(tampered); got != 0 {
		t.Errorf("tampered userID should return 0, got %d", got)
	}
}

func TestVerifyCookie_WrongSecret(t *testing.T) {
	h1 := &Handler{cookieSecret: []byte("secret-one-32-bytes-long-xxxxxx!")}
	h2 := &Handler{cookieSecret: []byte("secret-two-32-bytes-long-xxxxxx!")}

	signed := h1.signCookie(42)
	if got := h2.verifyCookie(signed); got != 0 {
		t.Errorf("different secret should return 0, got %d", got)
	}
}

func TestVerifyCookie_Expired(t *testing.T) {
	h := newTestHandler()
	expired := TestSignCookie(42, -10, h.cookieSecret)
	if got := h.verifyCookie(expired); got != 0 {
		t.Errorf("expired cookie should return 0, got %d", got)
	}
}

func TestVerifyCookie_NotYetExpired(t *testing.T) {
	h := newTestHandler()
	valid := TestSignCookie(42, 3600, h.cookieSecret)
	if got := h.verifyCookie(valid); got != 42 {
		t.Errorf("valid cookie should return 42, got %d", got)
	}
}

func TestVerifyCookie_MalformedInputs(t *testing.T) {
	h := newTestHandler()

	tests := []struct {
		name  string
		value string
	}{
		{"empty string", ""},
		{"no dots", "nodots"},
		{"one dot", "42.abc"},
		{"non-numeric userID", "abc.123.deadbeef"},
		{"zero userID", "0.9999999999.deadbeef"},
		{"negative userID", "-1.9999999999.deadbeef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.verifyCookie(tt.value); got != 0 {
				t.Errorf("verifyCookie(%q) = %d, want 0", tt.value, got)
			}
		})
	}
}

func TestVerifyCookie_ExportedMatchesMethod(t *testing.T) {
	h := newTestHandler()
	signed := h.signCookie(42)

	// The exported VerifyCookie and the method should agree
	got1 := h.verifyCookie(signed)
	got2 := VerifyCookie(signed, h.cookieSecret)
	if got1 != got2 {
		t.Errorf("method returned %d, exported function returned %d", got1, got2)
	}
}

func TestTimeGateToken_Format(t *testing.T) {
	h := newTestHandler()
	token := h.timeGateToken()

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("timeGateToken should have 2 dot-separated parts, got %d", len(parts))
	}
	if len(parts[0]) < 10 {
		t.Errorf("timestamp part too short: %q", parts[0])
	}
	if len(parts[1]) != 64 {
		t.Errorf("signature should be 64 hex chars, got %d", len(parts[1]))
	}
}

func TestTimeGateToken_RejectsImmediate(t *testing.T) {
	h := newTestHandler()
	token := h.timeGateToken()

	if h.verifyTimeGate(token) {
		t.Error("time gate should reject immediate verification (< 3 seconds)")
	}
}

func TestVerifyTimeGate_TamperedToken(t *testing.T) {
	h := newTestHandler()
	token := h.timeGateToken()

	tampered := token[:len(token)-1] + "x"
	if h.verifyTimeGate(tampered) {
		t.Error("tampered time gate token should be rejected")
	}
}

func TestVerifyTimeGate_MalformedInputs(t *testing.T) {
	h := newTestHandler()

	tests := []string{"", "nodot", "abc.def", ".sig"}
	for _, input := range tests {
		if h.verifyTimeGate(input) {
			t.Errorf("verifyTimeGate(%q) should return false", input)
		}
	}
}

func TestGenerateDeviceID(t *testing.T) {
	id := generateDeviceID()

	if len(id) != 32 {
		t.Errorf("generateDeviceID() length = %d, want 32", len(id))
	}

	id2 := generateDeviceID()
	if id == id2 {
		t.Error("two calls to generateDeviceID() returned the same value")
	}

	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("generateDeviceID() contains non-hex char: %c", c)
			break
		}
	}
}
