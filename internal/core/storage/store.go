// Package storage owns the platform's SQLite database: connection setup,
// platform-level migrations (users, sessions, agents, agent_tokens,
// platform_settings), and a small key-value settings helper.
//
// The driver is modernc.org/sqlite — a pure-Go SQLite implementation — so the
// project builds without CGO. Modules run their own migrations on the same DB
// handle returned by [OpenDB] and must namespace their tables with their
// module ID prefix (see [platform.Module]).
package storage

import (
	"database/sql"

	"github.com/hamed0406/biglybigly/internal/core/config"
	_ "modernc.org/sqlite"
)

// OpenDB opens (or creates) the SQLite database at cfg.DBPath, enables WAL
// mode for read concurrency, and pins MaxOpenConns to 1. The single-writer
// limit is deliberate: modernc.org/sqlite serializes writes anyway, and
// allowing multiple connections leads to spurious "database is locked" errors.
func OpenDB(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, err
	}

	// SQLite in pure-Go mode needs a single writer to avoid "database is locked"
	db.SetMaxOpenConns(1)

	// Enable WAL mode for better read concurrency
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

// RunMigrations creates the platform-level tables (users, sessions,
// platform_settings) if they do not already exist. Idempotent — safe to run
// on every startup. Module tables are migrated separately by the registry.
func RunMigrations(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			email    TEXT NOT NULL UNIQUE,
			created_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS sessions (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id  INTEGER NOT NULL,
			token    TEXT NOT NULL UNIQUE,
			created_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS platform_settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

// GetSetting reads a single value from platform_settings. Returns ("", nil)
// when the key is absent, so callers can treat "missing" and "empty" alike.
func GetSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM platform_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSetting upserts a single value into platform_settings.
func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		"INSERT INTO platform_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

// IsSetupComplete reports whether the first-run web setup wizard has been
// finished. Used by main to decide between bootstrap-token mode and normal
// operation, and by API handlers to gate the /api/setup/complete endpoint.
func IsSetupComplete(db *sql.DB) bool {
	val, err := GetSetting(db, "setup_complete")
	return err == nil && val == "true"
}
