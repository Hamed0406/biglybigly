package dnsfilter

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/hamed0406/biglybigly/internal/platform"
)

type Module struct {
	p         platform.Platform
	blocklist *BlocklistManager
	proxy     *Proxy
}

func New() *Module {
	return &Module{}
}

func (m *Module) ID() string      { return "dnsfilter" }
func (m *Module) Name() string    { return "DNS Filter" }
func (m *Module) Version() string { return "0.1.0" }
func (m *Module) Icon() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z"/><path d="M2 12h20"/><path d="M12 2c2.5 2.8 4 6.2 4 10s-1.5 7.2-4 10"/><path d="M12 2c-2.5 2.8-4 6.2-4 10s1.5 7.2 4 10"/><line x1="4" y1="4" x2="20" y2="20" stroke-width="3"/></svg>`
}

func (m *Module) Migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS dnsfilter_queries (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_name  TEXT NOT NULL DEFAULT 'local',
			domain      TEXT NOT NULL,
			qtype       TEXT NOT NULL DEFAULT 'A',
			client_ip   TEXT NOT NULL DEFAULT '127.0.0.1',
			blocked     INTEGER NOT NULL DEFAULT 0,
			upstream_ms INTEGER NOT NULL DEFAULT 0,
			answer      TEXT NOT NULL DEFAULT '',
			timestamp   INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_dnsfilter_queries_time
			ON dnsfilter_queries(timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_dnsfilter_queries_domain
			ON dnsfilter_queries(domain, timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_dnsfilter_queries_agent
			ON dnsfilter_queries(agent_name, timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_dnsfilter_queries_blocked
			ON dnsfilter_queries(blocked, timestamp DESC);

		CREATE TABLE IF NOT EXISTS dnsfilter_blocklists (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			url          TEXT NOT NULL UNIQUE,
			name         TEXT NOT NULL DEFAULT '',
			enabled      INTEGER NOT NULL DEFAULT 1,
			entry_count  INTEGER NOT NULL DEFAULT 0,
			last_updated INTEGER NOT NULL DEFAULT 0,
			created_at   INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS dnsfilter_custom_rules (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			domain     TEXT NOT NULL,
			action     TEXT NOT NULL DEFAULT 'block',
			created_at INTEGER NOT NULL,
			UNIQUE(domain, action)
		);

		CREATE TABLE IF NOT EXISTS dnsfilter_settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

func (m *Module) Init(p platform.Platform) error {
	m.p = p
	mux := p.Mux()
	auth := p.Auth()

	// Dashboard & query log
	mux.Handle("GET /api/dnsfilter/stats", auth(http.HandlerFunc(m.handleStats)))
	mux.Handle("GET /api/dnsfilter/queries", auth(http.HandlerFunc(m.handleQueries)))
	mux.Handle("GET /api/dnsfilter/agents", auth(http.HandlerFunc(m.handleAgents)))

	// Blocklist management
	mux.Handle("GET /api/dnsfilter/blocklists", auth(http.HandlerFunc(m.handleListBlocklists)))
	mux.Handle("POST /api/dnsfilter/blocklists", auth(http.HandlerFunc(m.handleAddBlocklist)))
	mux.Handle("DELETE /api/dnsfilter/blocklists/{id}", auth(http.HandlerFunc(m.handleDeleteBlocklist)))
	mux.Handle("POST /api/dnsfilter/blocklists/refresh", auth(http.HandlerFunc(m.handleRefreshBlocklists)))

	// Custom rules
	mux.Handle("GET /api/dnsfilter/rules", auth(http.HandlerFunc(m.handleListRules)))
	mux.Handle("POST /api/dnsfilter/rules", auth(http.HandlerFunc(m.handleAddRule)))
	mux.Handle("DELETE /api/dnsfilter/rules/{id}", auth(http.HandlerFunc(m.handleDeleteRule)))

	// Agent ingest (no auth — agents submit logs)
	mux.Handle("POST /api/dnsfilter/ingest", http.HandlerFunc(m.handleIngest))

	p.Log().Info("DNS Filter routes registered")

	return nil
}

func (m *Module) Start(ctx context.Context) error {
	// Server mode: seed default blocklists if none exist, run cleanup
	db := m.p.DB()
	logger := m.p.Log()

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM dnsfilter_blocklists`).Scan(&count)
	if count == 0 {
		now := currentTimestamp()
		defaults := []struct{ url, name string }{
			{"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts", "Steven Black Unified"},
		}
		for _, d := range defaults {
			db.Exec(`INSERT OR IGNORE INTO dnsfilter_blocklists (url, name, enabled, created_at) VALUES (?, ?, 1, ?)`,
				d.url, d.name, now)
		}
		logger.Info("DNS Filter: seeded default blocklists", "count", len(defaults))
	}

	// Run query cleanup every hour (keep 7 days)
	go m.runCleanup(ctx)

	<-ctx.Done()
	return nil
}

func (m *Module) AgentCapable() bool {
	return true
}

func (m *Module) AgentStart(ctx context.Context, conn platform.AgentConn) error {
	<-ctx.Done()
	return nil
}

func currentTimestamp() int64 {
	return timeNow().Unix()
}
