package urlcheck

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestMigrate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := New()
	if err := m.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Verify tables exist
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='urlcheck_urls'`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify urlcheck_urls table: %v", err)
	}
	if count != 1 {
		t.Fatal("urlcheck_urls table not created")
	}

	err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='urlcheck_history'`).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to verify urlcheck_history table: %v", err)
	}
	if count != 1 {
		t.Fatal("urlcheck_history table not created")
	}
}

func TestAddAndListURLs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := New()
	if err := m.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Add URL
	now := time.Now().Unix()
	result, err := db.Exec(`
		INSERT INTO urlcheck_urls (url, created_at, updated_at)
		VALUES (?, ?, ?)
	`, "https://example.com", now, now)
	if err != nil {
		t.Fatalf("Failed to insert URL: %v", err)
	}

	id, _ := result.LastInsertId()

	// Verify it exists
	var url string
	err = db.QueryRow(`SELECT url FROM urlcheck_urls WHERE id = ?`, id).Scan(&url)
	if err != nil {
		t.Fatalf("Failed to query URL: %v", err)
	}
	if url != "https://example.com" {
		t.Fatalf("Expected https://example.com, got %s", url)
	}
}

func TestCheckHistory(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := New()
	if err := m.Migrate(db); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Add URL
	now := time.Now().Unix()
	result, err := db.Exec(`
		INSERT INTO urlcheck_urls (url, created_at, updated_at)
		VALUES (?, ?, ?)
	`, "https://example.com", now, now)
	if err != nil {
		t.Fatalf("Failed to insert URL: %v", err)
	}

	urlID, _ := result.LastInsertId()

	// Add history entry
	_, err = db.Exec(`
		INSERT INTO urlcheck_history (url_id, status, response_time, error, checked_at)
		VALUES (?, ?, ?, ?, ?)
	`, urlID, 200, 100, "", now)
	if err != nil {
		t.Fatalf("Failed to insert history: %v", err)
	}

	// Query history
	var status int
	var responseTime int64
	err = db.QueryRow(`
		SELECT status, response_time FROM urlcheck_history WHERE url_id = ?
	`, urlID).Scan(&status, &responseTime)
	if err != nil {
		t.Fatalf("Failed to query history: %v", err)
	}
	if status != 200 {
		t.Fatalf("Expected status 200, got %d", status)
	}
	if responseTime != 100 {
		t.Fatalf("Expected response time 100, got %d", responseTime)
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test DB: %v", err)
	}
	return db
}
