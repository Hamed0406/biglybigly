package storage

import (
	"database/sql"

	"github.com/hamed0406/biglybigly/internal/core/config"
	_ "modernc.org/sqlite"
)

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

// GetSetting reads a single setting from the database
func GetSetting(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM platform_settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// SetSetting writes a single setting to the database
func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		"INSERT INTO platform_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

// IsSetupComplete checks if the first-run setup has been completed
func IsSetupComplete(db *sql.DB) bool {
	val, err := GetSetting(db, "setup_complete")
	return err == nil && val == "true"
}
