package config

import (
	"os"
	"strconv"
)

// Config holds application configuration from environment variables.
type Config struct {
	Port           int
	DBPath         string
	GTFSDir        string
	GTFSURL        string
	NexTripBaseURL string
	TestMode       bool
	ImportGTFS     bool // CLI flag: force GTFS re-import

	CookieSecret    string // HMAC key for signing session cookies
	MaxUsers        int    // Maximum number of registered users (0 = unlimited)
	MaxDevicesTotal int    // Absolute cap on devices per user (oldest evicted)
	MaxDevicesRecent int   // Max devices per user in rolling window
	DeviceWindowMin int    // Rolling window size in minutes
}

// Load reads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		Port:           envInt("GOBUS_PORT", 8080),
		DBPath:         envStr("GOBUS_DB_PATH", "./gobus.db"),
		GTFSDir:        envStr("GOBUS_GTFS_DIR", "./data"),
		GTFSURL:        envStr("GOBUS_GTFS_URL", "https://svc.metrotransit.org/mtgtfs/gtfs.zip"),
		NexTripBaseURL: envStr("GOBUS_NEXTRIP_URL", "https://svc.metrotransit.org/nextrip"),
		TestMode:       envBool("GOBUS_TEST_MODE", false),
		CookieSecret:    envStr("GOBUS_COOKIE_SECRET", ""),
		MaxUsers:        envInt("GOBUS_MAX_USERS", 100),
		MaxDevicesTotal: envInt("GOBUS_MAX_DEVICES_TOTAL", 5),
		MaxDevicesRecent: envInt("GOBUS_MAX_DEVICES_RECENT", 3),
		DeviceWindowMin: envInt("GOBUS_DEVICE_WINDOW_MIN", 10),
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
