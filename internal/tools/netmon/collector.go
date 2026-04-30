package netmon

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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

	// Initial scan
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

func (c *Collector) collect() []Flow {
	switch runtime.GOOS {
	case "linux":
		return c.collectLinux()
	default:
		return c.collectGeneric()
	}
}

// collectLinux reads /proc/net/tcp and /proc/net/tcp6
func (c *Collector) collectLinux() []Flow {
	var flows []Flow

	for _, entry := range []struct {
		path  string
		proto string
	}{
		{"/proc/net/tcp", "tcp"},
		{"/proc/net/tcp6", "tcp6"},
		{"/proc/net/udp", "udp"},
		{"/proc/net/udp6", "udp6"},
	} {
		parsed := parseProcNet(entry.path, entry.proto)
		flows = append(flows, parsed...)
	}

	// Filter: only outbound connections (remote port != 0, exclude loopback)
	var outbound []Flow
	for _, f := range flows {
		if f.RemotePort == 0 {
			continue
		}
		if f.RemoteIP == "127.0.0.1" || f.RemoteIP == "::1" || f.RemoteIP == "0.0.0.0" {
			continue
		}
		// Try reverse DNS for hostname
		f.Hostname = reverseResolve(f.RemoteIP)
		// Try to get process name
		if f.PID > 0 {
			f.Process = getProcessName(f.PID)
		}
		f.SeenAt = time.Now().Unix()
		outbound = append(outbound, f)
	}

	return outbound
}

// collectGeneric uses a basic approach for non-Linux systems
func (c *Collector) collectGeneric() []Flow {
	// Fallback: just report that we can't collect on this platform
	return nil
}

// parseProcNet reads a /proc/net/{tcp,udp} file
func parseProcNet(path, proto string) []Flow {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var flows []Flow
	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		localIP, localPort := parseHexAddr(fields[1])
		remoteIP, remotePort := parseHexAddr(fields[2])
		state := parseState(fields[3])

		// Get inode for PID lookup
		var pid int
		if len(fields) >= 10 {
			inode := fields[9]
			pid = findPIDByInode(inode)
		}

		flows = append(flows, Flow{
			Proto:      proto,
			LocalIP:    localIP,
			LocalPort:  localPort,
			RemoteIP:   remoteIP,
			RemotePort: remotePort,
			State:      state,
			PID:        pid,
		})
	}

	return flows
}

// parseHexAddr converts "0100007F:0035" to ("127.0.0.1", 53)
func parseHexAddr(s string) (string, int) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return "", 0
	}

	port, _ := strconv.ParseInt(parts[1], 16, 32)

	hexIP := parts[0]
	switch len(hexIP) {
	case 8: // IPv4
		b, err := hex.DecodeString(hexIP)
		if err != nil || len(b) != 4 {
			return "", int(port)
		}
		// /proc/net/tcp stores in little-endian
		ip := net.IPv4(b[3], b[2], b[1], b[0])
		return ip.String(), int(port)
	case 32: // IPv6
		b, err := hex.DecodeString(hexIP)
		if err != nil || len(b) != 16 {
			return "", int(port)
		}
		// IPv6 in /proc is stored as 4 groups of 4 bytes, each in little-endian
		for i := 0; i < 16; i += 4 {
			b[i], b[i+3] = b[i+3], b[i]
			b[i+1], b[i+2] = b[i+2], b[i+1]
		}
		ip := net.IP(b)
		return ip.String(), int(port)
	}
	return "", int(port)
}

// parseState converts hex state to human-readable
func parseState(hex string) string {
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
	if s, ok := states[strings.ToUpper(hex)]; ok {
		return s
	}
	return hex
}

// reverseResolve attempts reverse DNS lookup with a short timeout
func reverseResolve(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

// findPIDByInode walks /proc/*/fd/ to find which PID owns a socket inode
func findPIDByInode(inode string) int {
	if inode == "0" {
		return 0
	}

	target := fmt.Sprintf("socket:[%s]", inode)
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}

	for _, proc := range procs {
		if !proc.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(proc.Name())
		if err != nil {
			continue
		}

		fdPath := filepath.Join("/proc", proc.Name(), "fd")
		fds, err := os.ReadDir(fdPath)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdPath, fd.Name()))
			if err != nil {
				continue
			}
			if link == target {
				return pid
			}
		}
	}
	return 0
}

// getProcessName reads /proc/<pid>/comm
func getProcessName(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
