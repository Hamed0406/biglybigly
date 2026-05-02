package netmon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// psConnection mirrors the JSON output of Get-NetTCPConnection /
// Get-NetUDPEndpoint when piped through ConvertTo-Json.
type psConnection struct {
	LocalAddress  string `json:"LocalAddress"`
	LocalPort     int    `json:"LocalPort"`
	RemoteAddress string `json:"RemoteAddress"`
	RemotePort    int    `json:"RemotePort"`
	State         string `json:"State,omitempty"`
	OwningProcess int    `json:"OwningProcess"`
}

// psProcess mirrors a single record from Get-Process | Select Id,ProcessName.
type psProcess struct {
	Id          int    `json:"Id"`
	ProcessName string `json:"ProcessName"`
}

// collectPlatform gathers connections via PowerShell's Get-NetTCPConnection
// and Get-NetUDPEndpoint cmdlets. If the TCP query returns zero rows
// (frequently the case when running unelevated under certain Windows builds)
// it falls back to parsing `netstat -ano`. PIDs are resolved to process
// names in a single Get-Process call.
func (c *Collector) collectPlatform() []Flow {
	var flows []Flow

	c.logger.Info("Windows collector: running Get-NetTCPConnection...")
	tcpFlows, tcpErr := collectPS("Get-NetTCPConnection | Select-Object LocalAddress,LocalPort,RemoteAddress,RemotePort,State,OwningProcess | ConvertTo-Json -Compress", "tcp")
	if tcpErr != nil {
		c.logger.Warn("Windows collector: TCP via PowerShell failed", "err", tcpErr)
	} else {
		c.logger.Info("Windows collector: TCP connections via PowerShell", "count", len(tcpFlows))
	}

	// If PowerShell TCP returned nothing, fall back to netstat
	if len(tcpFlows) == 0 {
		c.logger.Info("Windows collector: trying netstat fallback...")
		netstatFlows, netstatErr := collectNetstat()
		if netstatErr != nil {
			c.logger.Warn("Windows collector: netstat fallback also failed", "err", netstatErr)
		} else {
			c.logger.Info("Windows collector: TCP connections via netstat", "count", len(netstatFlows))
			tcpFlows = netstatFlows
		}
	}

	if len(tcpFlows) == 0 {
		c.logger.Warn("Windows collector: 0 TCP connections from all methods — try running as Administrator")
	}
	flows = append(flows, tcpFlows...)

	c.logger.Info("Windows collector: running Get-NetUDPEndpoint...")
	udpFlows, udpErr := collectPS("Get-NetUDPEndpoint | Select-Object LocalAddress,LocalPort,@{N='RemoteAddress';E={'0.0.0.0'}},@{N='RemotePort';E={0}},@{N='State';E={'LISTEN'}},OwningProcess | ConvertTo-Json -Compress", "udp")
	if udpErr != nil {
		c.logger.Warn("Windows collector: UDP collection failed", "err", udpErr)
	} else {
		c.logger.Info("Windows collector: UDP endpoints", "count", len(udpFlows))
	}
	flows = append(flows, udpFlows...)

	// Resolve PIDs to process names
	pidMap := resolveProcessNames(flows)
	for i := range flows {
		if name, ok := pidMap[flows[i].PID]; ok {
			flows[i].Process = name
		}
	}

	c.logger.Info("Windows collector: total raw flows", "count", len(flows))
	return flows
}

// collectPS runs a PowerShell snippet that emits ConvertTo-Json output for
// connection records and parses it into Flow values.
func collectPS(script, proto string) ([]Flow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("powershell exit %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("powershell exec: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("powershell returned empty output")
	}

	slog.Debug("PowerShell raw output", "proto", proto, "bytes", len(out))
	return parsePSConnections(out, proto), nil
}

// collectNetstat is the fallback path when Get-NetTCPConnection produces no
// rows. `netstat -ano` works without elevation on every supported Windows
// version.
func collectNetstat() ([]Flow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "netstat", "-ano", "-p", "TCP").Output()
	if err != nil {
		return nil, fmt.Errorf("netstat: %w", err)
	}

	return parseNetstat(string(out)), nil
}

// parseNetstat parses `netstat -ano -p TCP` output. Each row has the form:
//
//	TCP    192.168.1.50:54321  142.250.74.46:443  ESTABLISHED  1234
func parseNetstat(output string) []Flow {
	var flows []Flow
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "TCP") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		localAddr := fields[1]
		remoteAddr := fields[2]
		state := fields[3]
		pid, _ := strconv.Atoi(fields[4])

		localIP, localPort := splitNetstatAddr(localAddr)
		remoteIP, remotePort := splitNetstatAddr(remoteAddr)

		flows = append(flows, Flow{
			Proto:      "tcp",
			LocalIP:    localIP,
			LocalPort:  localPort,
			RemoteIP:   remoteIP,
			RemotePort: remotePort,
			State:      mapWindowsState(state),
			PID:        pid,
		})
	}
	return flows
}

// splitNetstatAddr splits "192.168.1.50:443" or "[::1]:443" into IP and port.
func splitNetstatAddr(addr string) (string, int) {
	// Handle IPv6 bracket notation [::1]:port
	if strings.HasPrefix(addr, "[") {
		bracket := strings.LastIndex(addr, "]")
		if bracket == -1 {
			return addr, 0
		}
		ip := addr[1:bracket]
		portStr := ""
		if bracket+2 < len(addr) {
			portStr = addr[bracket+2:]
		}
		port, _ := strconv.Atoi(portStr)
		return ip, port
	}
	// IPv4: find last colon
	lastColon := strings.LastIndex(addr, ":")
	if lastColon == -1 {
		return addr, 0
	}
	ip := addr[:lastColon]
	port, _ := strconv.Atoi(addr[lastColon+1:])
	return ip, port
}

// parsePSConnections decodes ConvertTo-Json output for a list of connections.
// PowerShell emits a single object (not array) when there is exactly one
// result, so both shapes are accepted.
func parsePSConnections(data []byte, proto string) []Flow {
	// PowerShell outputs a single object (not array) if there's only one result
	var conns []psConnection
	if err := json.Unmarshal(data, &conns); err != nil {
		var single psConnection
		if err := json.Unmarshal(data, &single); err != nil {
			return nil
		}
		conns = []psConnection{single}
	}

	var flows []Flow
	for _, conn := range conns {
		flows = append(flows, Flow{
			Proto:      proto,
			LocalIP:    conn.LocalAddress,
			LocalPort:  conn.LocalPort,
			RemoteIP:   conn.RemoteAddress,
			RemotePort: conn.RemotePort,
			State:      mapWindowsState(conn.State),
			PID:        conn.OwningProcess,
		})
	}
	return flows
}

// resolveProcessNames maps PIDs to ProcessName using a single Get-Process
// call, avoiding one PowerShell invocation per flow.
func resolveProcessNames(flows []Flow) map[int]string {
	pids := make(map[int]bool)
	for _, f := range flows {
		if f.PID > 0 {
			pids[f.PID] = true
		}
	}
	if len(pids) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		"Get-Process | Select-Object Id,ProcessName | ConvertTo-Json -Compress").Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	var procs []psProcess
	if err := json.Unmarshal(out, &procs); err != nil {
		return nil
	}

	result := make(map[int]string)
	for _, p := range procs {
		if pids[p.Id] {
			result[p.Id] = p.ProcessName
		}
	}
	return result
}

// mapWindowsState translates Get-NetTCPConnection's CamelCase state names to
// the SCREAMING_SNAKE_CASE form used by the rest of the platform.
func mapWindowsState(state string) string {
	switch state {
	case "Established":
		return "ESTABLISHED"
	case "Listen":
		return "LISTEN"
	case "TimeWait":
		return "TIME_WAIT"
	case "CloseWait":
		return "CLOSE_WAIT"
	case "FinWait1":
		return "FIN_WAIT1"
	case "FinWait2":
		return "FIN_WAIT2"
	case "SynSent":
		return "SYN_SENT"
	case "SynReceived":
		return "SYN_RECV"
	case "LastAck":
		return "LAST_ACK"
	case "Closing":
		return "CLOSING"
	case "Bound":
		return "BOUND"
	default:
		return state
	}
}
