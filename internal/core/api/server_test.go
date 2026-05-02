package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hamed0406/biglybigly/internal/core/storage"
	"github.com/hamed0406/biglybigly/internal/platform"
	_ "modernc.org/sqlite"
)

func setupTestServer(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if err := storage.RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	// Create module tables that dashboard queries reference
	db.Exec(`CREATE TABLE IF NOT EXISTS sysmon_snapshots (
		id INTEGER PRIMARY KEY, agent_name TEXT, os TEXT,
		cpu_percent REAL, mem_used INTEGER, mem_total INTEGER,
		uptime_secs INTEGER, collected_at INTEGER)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS dnsfilter_queries (
		id INTEGER PRIMARY KEY, domain TEXT, type TEXT,
		blocked INTEGER, agent_name TEXT, client TEXT, timestamp INTEGER)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS dnsfilter_blocklists (
		id INTEGER PRIMARY KEY, name TEXT, url TEXT,
		enabled INTEGER DEFAULT 1, entry_count INTEGER DEFAULT 0)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS dnsfilter_custom_rules (
		id INTEGER PRIMARY KEY, domain TEXT, action TEXT)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS netmon_flows (
		id INTEGER PRIMARY KEY, agent_name TEXT, proto TEXT,
		remote_ip TEXT, remote_port INTEGER, last_seen INTEGER)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS urlcheck_urls (
		id INTEGER PRIMARY KEY, url TEXT, last_status INTEGER DEFAULT 0,
		last_check INTEGER DEFAULT 0, created_at INTEGER, updated_at INTEGER)`)

	mux := http.NewServeMux()
	logger := slog.Default()
	plat := platform.NewPlatform(db, mux, logger)
	reg := platform.NewRegistry(db, logger)
	handler := NewServer(plat, reg, "test-token-123")
	return handler, db
}

func TestSetupStatus(t *testing.T) {
	handler, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["setup_complete"] != false {
		t.Errorf("expected setup_complete=false, got %v", resp["setup_complete"])
	}
}

func TestSetupCompleteNoToken(t *testing.T) {
	handler, _ := setupTestServer(t)

	body := `{"mode":"server"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/complete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestSetupCompleteInvalidMode(t *testing.T) {
	handler, _ := setupTestServer(t)

	tests := []struct {
		name string
		body string
	}{
		{"empty mode", `{"mode":""}`},
		{"invalid mode", `{"mode":"standalone"}`},
		{"typo mode", `{"mode":"Server"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/setup/complete", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Bootstrap-Token", "test-token-123")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSetupCompleteSuccess(t *testing.T) {
	handler, _ := setupTestServer(t)

	// Complete setup with server mode
	body := `{"mode":"server","instance_name":"test-instance"}`
	req := httptest.NewRequest(http.MethodPost, "/api/setup/complete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Bootstrap-Token", "test-token-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp["ok"])
	}
	if resp["mode"] != "server" {
		t.Errorf("expected mode=server, got %v", resp["mode"])
	}

	// Verify status now shows complete
	req = httptest.NewRequest(http.MethodGet, "/api/setup/status", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var status map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status["setup_complete"] != true {
		t.Errorf("expected setup_complete=true after setup, got %v", status["setup_complete"])
	}
	if status["mode"] != "server" {
		t.Errorf("expected mode=server, got %v", status["mode"])
	}
	if status["instance_name"] != "test-instance" {
		t.Errorf("expected instance_name=test-instance, got %v", status["instance_name"])
	}
}

func TestModulesEndpoint(t *testing.T) {
	handler, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/modules", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var modules []interface{}
	if err := json.NewDecoder(rec.Body).Decode(&modules); err != nil {
		t.Fatal(err)
	}
	if len(modules) != 0 {
		t.Errorf("expected empty array, got %d modules", len(modules))
	}
}

func TestDashboardEmpty(t *testing.T) {
	handler, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var dash map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&dash); err != nil {
		t.Fatal(err)
	}

	// Verify zeroed numeric fields
	expectedZero := []string{"agent_count", "agents_online", "dns_total", "dns_blocked", "dns_blocked_pct", "blocklist_size", "net_flows"}
	for _, field := range expectedZero {
		val, ok := dash[field]
		if !ok {
			t.Errorf("missing field %s", field)
			continue
		}
		if val.(float64) != 0 {
			t.Errorf("expected %s=0, got %v", field, val)
		}
	}

	// Verify empty arrays
	expectedArrays := []string{"top_blocked", "top_queried", "agents", "urls_down", "recent_blocks"}
	for _, field := range expectedArrays {
		val, ok := dash[field]
		if !ok {
			t.Errorf("missing field %s", field)
			continue
		}
		arr, ok := val.([]interface{})
		if !ok {
			t.Errorf("expected %s to be array, got %T", field, val)
			continue
		}
		if len(arr) != 0 {
			t.Errorf("expected %s to be empty, got %d items", field, len(arr))
		}
	}
}

func TestDashboardWithData(t *testing.T) {
	handler, db := setupTestServer(t)

	now := time.Now().Unix()

	// Insert sysmon data (2 agents, 1 online)
	db.Exec(`INSERT INTO sysmon_snapshots (agent_name, os, cpu_percent, mem_used, mem_total, uptime_secs, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "agent1", "linux", 45.5, 4000000000, 8000000000, 3600, now)
	db.Exec(`INSERT INTO sysmon_snapshots (agent_name, os, cpu_percent, mem_used, mem_total, uptime_secs, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "agent2", "windows", 20.0, 2000000000, 4000000000, 7200, now-600)

	// Insert DNS queries (3 total, 2 blocked)
	db.Exec(`INSERT INTO dnsfilter_queries (domain, type, blocked, agent_name, client, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`, "ads.example.com", "A", 1, "agent1", "192.168.1.10", now)
	db.Exec(`INSERT INTO dnsfilter_queries (domain, type, blocked, agent_name, client, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`, "tracker.example.com", "A", 1, "agent1", "192.168.1.10", now-60)
	db.Exec(`INSERT INTO dnsfilter_queries (domain, type, blocked, agent_name, client, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`, "google.com", "A", 0, "agent1", "192.168.1.10", now-120)

	// Insert blocklist with entries
	db.Exec(`INSERT INTO dnsfilter_blocklists (name, url, enabled, entry_count) VALUES (?, ?, 1, ?)`, "test-list", "http://example.com/list.txt", 5000)

	// Insert network flows
	db.Exec(`INSERT INTO netmon_flows (agent_name, proto, remote_ip, remote_port, last_seen) VALUES (?, ?, ?, ?, ?)`, "agent1", "tcp", "8.8.8.8", 443, now)
	db.Exec(`INSERT INTO netmon_flows (agent_name, proto, remote_ip, remote_port, last_seen) VALUES (?, ?, ?, ?, ?)`, "agent1", "tcp", "1.1.1.1", 80, now-30)

	// Insert a URL that is down
	db.Exec(`INSERT INTO urlcheck_urls (url, last_status, last_check, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, "https://down.example.com", 503, now-60, now-3600, now-60)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var dash map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&dash); err != nil {
		t.Fatal(err)
	}

	// Verify agent counts
	if int(dash["agent_count"].(float64)) != 2 {
		t.Errorf("expected agent_count=2, got %v", dash["agent_count"])
	}
	if int(dash["agents_online"].(float64)) != 1 {
		t.Errorf("expected agents_online=1, got %v", dash["agents_online"])
	}

	// Verify DNS stats
	if int(dash["dns_total"].(float64)) != 3 {
		t.Errorf("expected dns_total=3, got %v", dash["dns_total"])
	}
	if int(dash["dns_blocked"].(float64)) != 2 {
		t.Errorf("expected dns_blocked=2, got %v", dash["dns_blocked"])
	}
	expectedPct := float64(2) / float64(3) * 100
	if got := dash["dns_blocked_pct"].(float64); got < expectedPct-0.1 || got > expectedPct+0.1 {
		t.Errorf("expected dns_blocked_pct≈%.1f, got %.1f", expectedPct, got)
	}

	// Verify blocklist size
	if int(dash["blocklist_size"].(float64)) != 5000 {
		t.Errorf("expected blocklist_size=5000, got %v", dash["blocklist_size"])
	}

	// Verify network flows
	if int(dash["net_flows"].(float64)) != 2 {
		t.Errorf("expected net_flows=2, got %v", dash["net_flows"])
	}

	// Verify top_blocked has entries
	topBlocked := dash["top_blocked"].([]interface{})
	if len(topBlocked) != 2 {
		t.Errorf("expected 2 top_blocked entries, got %d", len(topBlocked))
	}

	// Verify agents list
	agents := dash["agents"].([]interface{})
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}

	// Verify urls_down
	urlsDown := dash["urls_down"].([]interface{})
	if len(urlsDown) != 1 {
		t.Errorf("expected 1 url down, got %d", len(urlsDown))
	}
	if len(urlsDown) > 0 {
		u := urlsDown[0].(map[string]interface{})
		if u["url"] != "https://down.example.com" {
			t.Errorf("expected down url=https://down.example.com, got %v", u["url"])
		}
		if int(u["status_code"].(float64)) != 503 {
			t.Errorf("expected status_code=503, got %v", u["status_code"])
		}
	}

	// Verify recent_blocks
	recentBlocks := dash["recent_blocks"].([]interface{})
	if len(recentBlocks) != 2 {
		t.Errorf("expected 2 recent blocks, got %d", len(recentBlocks))
	}
}
