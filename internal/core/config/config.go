// Package config loads platform configuration from environment variables.
// Configuration is intentionally env-var driven (no flag parsing here) so the
// same binary behaves identically in Docker, systemd, and local-dev contexts.
package config

import (
	"os"
)

// Config holds the platform-level settings needed to bootstrap the process.
// Module-specific settings live elsewhere (see [platform.ModuleConfig]).
type Config struct {
	// HTTPAddr is the listen address for the HTTP server (e.g. ":8082").
	HTTPAddr string
	// DBPath is the filesystem path to the SQLite database file.
	DBPath string
	// BaseURL is the externally reachable root URL, used for OAuth redirects
	// and other absolute-URL construction.
	BaseURL string
}

// Load reads configuration from BIGLYBIGLY_* environment variables, falling
// back to sensible defaults for local development.
func Load() *Config {
	return &Config{
		HTTPAddr: getEnv("BIGLYBIGLY_HTTP_ADDR", ":8082"),
		DBPath:   getEnv("BIGLYBIGLY_DB_PATH", "./biglybigly.db"),
		BaseURL:  getEnv("BIGLYBIGLY_BASE_URL", "http://localhost:8082"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
