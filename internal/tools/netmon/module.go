// Package netmon implements passive network connection monitoring.
//
// It periodically reads the OS connection table (/proc/net/{tcp,udp}* on
// Linux, lsof on macOS, Get-NetTCPConnection / netstat on Windows) and
// records each unique flow keyed by (agent, proto, remote_ip, remote_port).
// Repeat sightings bump a counter and update last_seen rather than inserting
// new rows. A background enricher resolves remote IPs to hostnames and
// maintains a per-agent IP↔hostname history for forensic lookups.
package netmon

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/hamed0406/biglybigly/internal/platform"
)

// Module is the netmon platform.Module implementation.
type Module struct {
	p platform.Platform
}

// New constructs an uninitialized netmon module.
func New() *Module {
	return &Module{}
}

// ID returns the stable module identifier used as the route and table prefix.
func (m *Module) ID() string      { return "netmon" }

// Name returns the human-readable module name shown in the UI.
func (m *Module) Name() string    { return "Network Monitor" }

// Version returns the module's semantic version.
func (m *Module) Version() string { return "0.1.0" }

// Icon returns the inline SVG icon rendered in the sidebar.
func (m *Module) Icon() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>`
}

// Migrate creates netmon_flows, netmon_dns_cache and netmon_hostname_history
// tables plus their indexes. Idempotent.
func (m *Module) Migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS netmon_flows (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			agent_name  TEXT NOT NULL DEFAULT 'local',
			proto       TEXT NOT NULL,
			local_ip    TEXT,
			local_port  INTEGER,
			remote_ip   TEXT NOT NULL,
			remote_port INTEGER NOT NULL,
			hostname    TEXT,
			pid         INTEGER,
			process     TEXT,
			state       TEXT,
			count       INTEGER NOT NULL DEFAULT 1,
			first_seen  INTEGER NOT NULL,
			last_seen   INTEGER NOT NULL,
			UNIQUE(agent_name, proto, remote_ip, remote_port)
		);
		CREATE INDEX IF NOT EXISTS idx_netmon_flows_remote
			ON netmon_flows(remote_ip, remote_port);
		CREATE INDEX IF NOT EXISTS idx_netmon_flows_hostname
			ON netmon_flows(hostname);
		CREATE INDEX IF NOT EXISTS idx_netmon_flows_last_seen
			ON netmon_flows(last_seen DESC);
		CREATE INDEX IF NOT EXISTS idx_netmon_flows_agent
			ON netmon_flows(agent_name, last_seen DESC);

		CREATE TABLE IF NOT EXISTS netmon_dns_cache (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			ip          TEXT NOT NULL UNIQUE,
			hostname    TEXT,
			resolved_at INTEGER NOT NULL
		);

		CREATE TABLE IF NOT EXISTS netmon_hostname_history (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			ip          TEXT NOT NULL,
			hostname    TEXT NOT NULL,
			agent_name  TEXT NOT NULL DEFAULT 'local',
			first_seen  INTEGER NOT NULL,
			last_seen   INTEGER NOT NULL,
			seen_count  INTEGER NOT NULL DEFAULT 1,
			UNIQUE(ip, hostname, agent_name)
		);
		CREATE INDEX IF NOT EXISTS idx_netmon_hostname_history_ip
			ON netmon_hostname_history(ip, last_seen DESC);
		CREATE INDEX IF NOT EXISTS idx_netmon_hostname_history_hostname
			ON netmon_hostname_history(hostname);
		CREATE INDEX IF NOT EXISTS idx_netmon_hostname_history_last_seen
			ON netmon_hostname_history(last_seen DESC);
		CREATE INDEX IF NOT EXISTS idx_netmon_hostname_history_agent
			ON netmon_hostname_history(agent_name);
	`)
	return err
}

// Init registers HTTP routes for flow listing, top-N stats, the topology
// graph, hostname history, and the agent ingest endpoint.
func (m *Module) Init(p platform.Platform) error {
	m.p = p
	mux := p.Mux()
	auth := p.Auth()
	logger := p.Log()

	mux.Handle("GET /api/netmon/flows", auth(http.HandlerFunc(m.handleListFlows)))
	mux.Handle("GET /api/netmon/top-hosts", auth(http.HandlerFunc(m.handleTopHosts)))
	mux.Handle("GET /api/netmon/top-ports", auth(http.HandlerFunc(m.handleTopPorts)))
	mux.Handle("GET /api/netmon/stats", auth(http.HandlerFunc(m.handleStats)))
	mux.Handle("GET /api/netmon/agents", auth(http.HandlerFunc(m.handleAgents)))
	mux.Handle("GET /api/netmon/graph", auth(http.HandlerFunc(m.handleGraph)))
	mux.Handle("GET /api/netmon/hostnames", auth(http.HandlerFunc(m.handleHostnames)))
	mux.Handle("GET /api/netmon/hostnames/recent", auth(http.HandlerFunc(m.handleRecentHostnames)))
	mux.Handle("GET /api/netmon/hostnames/lookup", auth(http.HandlerFunc(m.handleHostnameLookup)))
	mux.Handle("GET /api/netmon/hostnames/stats", auth(http.HandlerFunc(m.handleHostnameStats)))
	mux.Handle("POST /api/netmon/ingest", http.HandlerFunc(m.handleIngest))

	logger.Info("Netmon routes registered",
		"endpoints", []string{"/api/netmon/flows", "/api/netmon/stats", "/api/netmon/ingest"},
	)

	return nil
}

// Start runs server-mode work: a local connection collector under the
// "local" agent name and the periodic hostname enricher. Blocks on ctx.
func (m *Module) Start(ctx context.Context) error {
	// In server mode, start local monitoring
	collector := NewCollector()
	go collector.Run(ctx, m.p.DB(), "local")

	// Start hostname history enrichment worker
	go m.runHostnameEnricher(ctx)

	<-ctx.Done()
	return nil
}

// AgentCapable reports that netmon collects data on remote agents.
func (m *Module) AgentCapable() bool {
	return true
}

// AgentStart runs the collector on the agent and ships flow batches over the
// platform AgentConn until ctx is cancelled.
func (m *Module) AgentStart(ctx context.Context, conn platform.AgentConn) error {
	collector := NewCollector()
	go collector.RunAndSend(ctx, conn)
	<-ctx.Done()
	return nil
}
