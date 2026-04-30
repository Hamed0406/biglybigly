package netmon

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/hamed0406/biglybigly/internal/platform"
)

type Module struct {
	p platform.Platform
}

func New() *Module {
	return &Module{}
}

func (m *Module) ID() string      { return "netmon" }
func (m *Module) Name() string    { return "Network Monitor" }
func (m *Module) Version() string { return "0.1.0" }
func (m *Module) Icon() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>`
}

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
	`)
	return err
}

func (m *Module) Init(p platform.Platform) error {
	m.p = p
	mux := p.Mux()
	auth := p.Auth()
	logger := p.Log()

	mux.Handle("GET /api/netmon/flows", auth(http.HandlerFunc(m.handleListFlows)))
	mux.Handle("GET /api/netmon/top-hosts", auth(http.HandlerFunc(m.handleTopHosts)))
	mux.Handle("GET /api/netmon/top-ports", auth(http.HandlerFunc(m.handleTopPorts)))
	mux.Handle("GET /api/netmon/stats", auth(http.HandlerFunc(m.handleStats)))
	mux.Handle("POST /api/netmon/ingest", http.HandlerFunc(m.handleIngest))

	logger.Info("Netmon routes registered",
		"endpoints", []string{"/api/netmon/flows", "/api/netmon/stats", "/api/netmon/ingest"},
	)

	return nil
}

func (m *Module) Start(ctx context.Context) error {
	// In server mode, start local monitoring if no agents connected
	collector := NewCollector()
	go collector.Run(ctx, m.p.DB(), "local")
	<-ctx.Done()
	return nil
}

func (m *Module) AgentCapable() bool {
	return true
}

func (m *Module) AgentStart(ctx context.Context, conn platform.AgentConn) error {
	collector := NewCollector()
	go collector.RunAndSend(ctx, conn)
	<-ctx.Done()
	return nil
}
