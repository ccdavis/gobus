package config

import (
	"os"
	"strconv"
)

// Config holds application configuration from environment variables.
type Config struct {
	Host           string // bind address; 127.0.0.1 = local-only, 0.0.0.0 = LAN
	Port           int
	DBPath         string
	GTFSDir        string
	GTFSURL        string
	NexTripBaseURL string
	TestMode       bool
	ImportGTFS     bool // CLI flag: force GTFS re-import
	// GeocodeEnabled allows network reverse geocoding (Nominatim) for the
	// nearby-page location label. When false, coordinates never leave the
	// device and a locally computed label (nearest stop) is used instead.
	// The native iPhone build forces this off.
	GeocodeEnabled bool
	// NativeShell is true when the server runs inside the native app shell
	// (iOS WKWebView). It disables browser-PWA behavior — manifest links,
	// install prompts, and service-worker registration — that make no sense
	// in an installed native app.
	NativeShell bool
}

// Load reads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		Host:           envStr("GOBUS_HOST", "127.0.0.1"),
		Port:           envInt("GOBUS_PORT", 8080),
		DBPath:         envStr("GOBUS_DB_PATH", "./gobus.db"),
		GTFSDir:        envStr("GOBUS_GTFS_DIR", "./data"),
		GTFSURL:        envStr("GOBUS_GTFS_URL", "https://svc.metrotransit.org/mtgtfs/gtfs.zip"),
		NexTripBaseURL: envStr("GOBUS_NEXTRIP_URL", "https://svc.metrotransit.org/nextrip"),
		TestMode:       envBool("GOBUS_TEST_MODE", false),
		GeocodeEnabled: envBool("GOBUS_GEOCODE", true),
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
