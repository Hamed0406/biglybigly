// Package sysmon collects per-host system metrics (CPU, memory, load,
// uptime, hostname, OS info, disk usage) from each connected agent and
// stores them as time-stamped snapshots. Agents push snapshots to the
// server's /api/sysmon/ingest endpoint; the UI renders the latest values
// and time-series history.
package sysmon

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/hamed0406/biglybigly/internal/platform"
)

// Module is the sysmon platform.Module implementation.
type Module struct {
	p platform.Platform
}

// New constructs an uninitialized sysmon module.
func New() *Module {
	return &Module{}
}

// ID returns the stable module identifier used as the route and table prefix.
func (m *Module) ID() string      { return "sysmon" }

// Name returns the human-readable module name shown in the UI.
func (m *Module) Name() string    { return "System Monitor" }

// Version returns the module's semantic version.
func (m *Module) Version() string { return "0.1.0" }

// Icon returns the inline SVG icon rendered in the sidebar.
func (m *Module) Icon() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8"/><path d="M12 17v4"/><path d="M7 10l3-3 2 2 4-4"/></svg>`
}

// Migrate creates sysmon_snapshots and sysmon_disks tables. Idempotent.
func (m *Module) Migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sysmon_snapshots (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_name    TEXT NOT NULL,
			cpu_percent   REAL NOT NULL DEFAULT 0,
			mem_total     INTEGER NOT NULL DEFAULT 0,
			mem_used      INTEGER NOT NULL DEFAULT 0,
			mem_available INTEGER NOT NULL DEFAULT 0,
			load1         REAL NOT NULL DEFAULT 0,
			load5         REAL NOT NULL DEFAULT 0,
			load15        REAL NOT NULL DEFAULT 0,
			os_info       TEXT NOT NULL DEFAULT '',
			hostname      TEXT NOT NULL DEFAULT '',
			uptime_secs   INTEGER NOT NULL DEFAULT 0,
			collected_at  INTEGER NOT NULL,
			UNIQUE(agent_name, collected_at)
		);
		CREATE INDEX IF NOT EXISTS idx_sysmon_snapshots_agent_time
			ON sysmon_snapshots(agent_name, collected_at DESC);
		CREATE INDEX IF NOT EXISTS idx_sysmon_snapshots_time
			ON sysmon_snapshots(collected_at);

		CREATE TABLE IF NOT EXISTS sysmon_disks (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id  INTEGER NOT NULL,
			agent_name   TEXT NOT NULL,
			mount_point  TEXT NOT NULL,
			fs_type      TEXT NOT NULL DEFAULT '',
			total_bytes  INTEGER NOT NULL DEFAULT 0,
			used_bytes   INTEGER NOT NULL DEFAULT 0,
			avail_bytes  INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (snapshot_id) REFERENCES sysmon_snapshots(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_sysmon_disks_agent
			ON sysmon_disks(agent_name);
		CREATE INDEX IF NOT EXISTS idx_sysmon_disks_snapshot
			ON sysmon_disks(snapshot_id);
	`)
	return err
}

// Init registers HTTP routes for current/history/disk views, the agent list,
// and the agent ingest endpoint.
func (m *Module) Init(p platform.Platform) error {
	m.p = p
	mux := p.Mux()
	auth := p.Auth()

	mux.Handle("GET /api/sysmon/current", auth(http.HandlerFunc(m.handleCurrent)))
	mux.Handle("GET /api/sysmon/history", auth(http.HandlerFunc(m.handleHistory)))
	mux.Handle("GET /api/sysmon/disks", auth(http.HandlerFunc(m.handleDisks)))
	mux.Handle("GET /api/sysmon/agents", auth(http.HandlerFunc(m.handleAgents)))
	mux.Handle("POST /api/sysmon/ingest", http.HandlerFunc(m.handleIngest))

	p.Log().Info("Sysmon routes registered",
		"endpoints", []string{"/api/sysmon/current", "/api/sysmon/history", "/api/sysmon/disks", "/api/sysmon/ingest"},
	)

	return nil
}

// Start launches the periodic 24h snapshot retention cleanup. Blocks on ctx.
func (m *Module) Start(ctx context.Context) error {
	// Start cleanup goroutine to remove old snapshots (keep 24h)
	go m.runCleanup(ctx)
	<-ctx.Done()
	return nil
}

// AgentCapable reports that sysmon collects data on remote agents.
func (m *Module) AgentCapable() bool {
	return true
}

// AgentStart is the agent-side entry point. Snapshot collection is driven
// by the agent runtime; this just blocks until shutdown.
func (m *Module) AgentStart(ctx context.Context, conn platform.AgentConn) error {
	<-ctx.Done()
	return nil
}
