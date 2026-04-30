package netmon

import (
	"context"
	"database/sql"
	"net"
	"strings"
	"time"

	"github.com/hamed0406/biglybigly/internal/platform"
)

// Flow represents a single observed network connection
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

// Collector polls the OS for active connections
type Collector struct {
	seen map[string]bool
}

func NewCollector() *Collector {
	return &Collector{
		seen: make(map[string]bool),
	}
}

// Run polls connections and stores them directly in the DB (server mode)
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

// RunAndSend polls connections and sends them to the server (agent mode)
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

// collect delegates to the platform-specific collectPlatform() implementation
func (c *Collector) collect() []Flow {
	raw := c.collectPlatform()
	return filterAndEnrich(raw)
}

// filterAndEnrich removes loopback/unconnected flows and adds reverse DNS
func filterAndEnrich(flows []Flow) []Flow {
	var outbound []Flow
	for _, f := range flows {
		if f.RemotePort == 0 {
			continue
		}
		if f.RemoteIP == "127.0.0.1" || f.RemoteIP == "::1" || f.RemoteIP == "0.0.0.0" {
			continue
		}
		f.Hostname = reverseResolve(f.RemoteIP)
		f.SeenAt = time.Now().Unix()
		outbound = append(outbound, f)
	}
	return outbound
}

// reverseResolve attempts reverse DNS lookup
func reverseResolve(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

// parseState converts hex state codes to human-readable names
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
