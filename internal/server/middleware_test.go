package server

import (
	"testing"

	"gobus/internal/handler"
)

var testSecret = []byte("test-secret-32-bytes-long-xxxxx!")

func TestVerifyCookie_Valid(t *testing.T) {
	cookie := handler.TestSignCookie(42, 3600, testSecret)
	got := handler.VerifyCookie(cookie, testSecret)
	if got != 42 {
		t.Errorf("VerifyCookie(valid) = %d, want 42", got)
	}
}

func TestVerifyCookie_Expired(t *testing.T) {
	cookie := handler.TestSignCookie(42, -10, testSecret)
	got := handler.VerifyCookie(cookie, testSecret)
	if got != 0 {
		t.Errorf("VerifyCookie(expired) = %d, want 0", got)
	}
}

func TestVerifyCookie_TamperedSig(t *testing.T) {
	cookie := handler.TestSignCookie(42, 3600, testSecret)
	tampered := cookie[:len(cookie)-1] + "x"
	got := handler.VerifyCookie(tampered, testSecret)
	if got != 0 {
		t.Errorf("VerifyCookie(tampered sig) = %d, want 0", got)
	}
}

func TestVerifyCookie_WrongSecret(t *testing.T) {
	cookie := handler.TestSignCookie(42, 3600, testSecret)
	wrongSecret := []byte("wrong-secret-32-bytes-long-xxxx!")
	got := handler.VerifyCookie(cookie, wrongSecret)
	if got != 0 {
		t.Errorf("VerifyCookie(wrong secret) = %d, want 0", got)
	}
}

func TestVerifyCookie_Malformed(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"no dots", "abc"},
		{"one dot", "42.abc"},
		{"non-numeric userID", "abc.123.sig"},
		{"zero userID", "0.9999999999.sig"},
		{"negative userID", "-1.9999999999.sig"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handler.VerifyCookie(tt.value, testSecret); got != 0 {
				t.Errorf("VerifyCookie(%q) = %d, want 0", tt.value, got)
			}
		})
	}
}
