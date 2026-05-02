package netmon

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/hamed0406/biglybigly/internal/platform"
)

// Flow represents a single observed network connection.
type Flow struct {
	Proto      string `json:"proto"`
	LocalIP    string `json:"local_ip,omitempty"`
	LocalPort  int    `json:"local_port,omitempty"`
	RemoteIP   string `json:"remote_ip"`
	RemotePort int    `json:"remote_port"`
	Hostname   string `json:"hostname,omitempty"`
	PID        int    `json:"pid,omitempty"`
	Process    string `json:"process,omitempty"`
	State      string `json:"state,omitempty"`
	SeenAt     int64  `json:"seen_at"`
}

// Collector polls the OS for active connections. The seen map is reserved
// for future per-cycle deduplication; current dedup happens at insert time
// via the (agent, proto, remote_ip, remote_port) UNIQUE constraint.
type Collector struct {
	seen   map[string]bool
	logger *slog.Logger
}

// NewCollector returns a Collector using the default slog logger.
func NewCollector() *Collector {
	return &Collector{
		seen:   make(map[string]bool),
		logger: slog.Default(),
	}
}

// NewCollectorWithLogger returns a Collector using the provided logger.
func NewCollectorWithLogger(logger *slog.Logger) *Collector {
	return &Collector{
		seen:   make(map[string]bool),
		logger: logger,
	}
}

// Run polls connections every 10s and writes them directly to the database
// (server / local mode). Blocks until ctx is cancelled.
func (c *Collector) Run(ctx context.Context, db *sql.DB, agentName string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	c.collectAndStore(db, agentName)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collectAndStore(db, agentName)
		}
	}
}

// RunAndSend polls connections every 30s and ships them to the server over
// the agent connection (agent mode). Blocks until ctx is cancelled.
func (c *Collector) RunAndSend(ctx context.Context, conn platform.AgentConn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			flows := c.collect()
			if len(flows) > 0 {
				conn.Send("stats", map[string]interface{}{
					"flows": flows,
				})
			}
		}
	}
}

// collectAndStore performs one collection cycle and upserts the rows. The
// ON CONFLICT clause coalesces repeat sightings into a single row, bumping
// count and last_seen, and preserves any non-null hostname/pid/process that
// has been previously enriched.
func (c *Collector) collectAndStore(db *sql.DB, agentName string) {
	flows := c.collect()
	now := time.Now().Unix()

	for _, f := range flows {
		_, err := db.Exec(`
			INSERT INTO netmon_flows (agent_name, proto, local_ip, local_port, remote_ip, remote_port, hostname, pid, process, state, count, first_seen, last_seen)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
			ON CONFLICT(agent_name, proto, remote_ip, remote_port)
			DO UPDATE SET
				count = count + 1,
				last_seen = ?,
				state = ?,
				hostname = COALESCE(excluded.hostname, hostname),
				pid = COALESCE(excluded.pid, pid),
				process = COALESCE(excluded.process, process)
		`, agentName, f.Proto, f.LocalIP, f.LocalPort, f.RemoteIP, f.RemotePort,
			f.Hostname, f.PID, f.Process, f.State, now, now, now, f.State)
		if err != nil {
			continue
		}
	}
}

// collect delegates to the platform-specific collectPlatform implementation
// and applies common filtering / rDNS enrichment.
func (c *Collector) collect() []Flow {
	raw := c.collectPlatform()
	c.logger.Debug("Platform collector returned", "raw_count", len(raw))
	filtered := filterAndEnrich(raw)
	c.logger.Debug("After filtering", "filtered_count", len(filtered))
	return filtered
}

// CollectFiltered is the public agent-mode entry point. Returns the same
// filtered flow list as collect() and logs a count at INFO level.
func (c *Collector) CollectFiltered() []Flow {
	flows := c.collect()
	if len(flows) == 0 {
		c.logger.Info("Collector: 0 flows after filtering (check platform collector)")
	} else {
		c.logger.Info("Collector: flows ready", "count", len(flows))
	}
	return flows
}

// filterAndEnrich drops loopback / wildcard / unconnected entries and adds
// reverse-DNS hostnames and a SeenAt timestamp.
func filterAndEnrich(flows []Flow) []Flow {
	var result []Flow
	for _, f := range flows {
		// For TCP: skip if no remote connection
		if f.Proto == "tcp" && f.RemotePort == 0 {
			continue
		}
		// Skip loopback and unbound addresses
		if f.RemoteIP == "127.0.0.1" || f.RemoteIP == "::1" {
			continue
		}
		// For TCP: skip unconnected; for UDP: skip if listening on wildcard with no remote
		if f.RemoteIP == "0.0.0.0" || f.RemoteIP == "::" {
			continue
		}
		f.Hostname = reverseResolve(f.RemoteIP)
		f.SeenAt = time.Now().Unix()
		result = append(result, f)
	}
	return result
}

// reverseResolve performs a best-effort reverse DNS lookup, returning "" if
// resolution fails.
func reverseResolve(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

// parseState maps Linux /proc/net TCP state hex codes to their human-readable
// names (ESTABLISHED, LISTEN, …); returns the input unchanged on miss.
func parseState(hexState string) string {
	states := map[string]string{
		"01": "ESTABLISHED",
		"02": "SYN_SENT",
		"03": "SYN_RECV",
		"04": "FIN_WAIT1",
		"05": "FIN_WAIT2",
		"06": "TIME_WAIT",
		"07": "CLOSE",
		"08": "CLOSE_WAIT",
		"09": "LAST_ACK",
		"0A": "LISTEN",
		"0B": "CLOSING",
	}
	if s, ok := states[strings.ToUpper(hexState)]; ok {
		return s
	}
	return hexState
}
