package netmon

import (
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

// psConnection mirrors the JSON output of Get-NetTCPConnection / Get-NetUDPEndpoint
type psConnection struct {
	LocalAddress  string `json:"LocalAddress"`
	LocalPort     int    `json:"LocalPort"`
	RemoteAddress string `json:"RemoteAddress"`
	RemotePort    int    `json:"RemotePort"`
	State         string `json:"State,omitempty"`
	OwningProcess int    `json:"OwningProcess"`
}

// psProcess mirrors Get-Process output
type psProcess struct {
	Id          int    `json:"Id"`
	ProcessName string `json:"ProcessName"`
}

// collectPlatform uses PowerShell Get-NetTCPConnection and Get-NetUDPEndpoint
// for structured, reliable output on Windows.
func (c *Collector) collectPlatform() []Flow {
	var flows []Flow

	// Collect TCP connections
	tcpFlows := collectPS("Get-NetTCPConnection | Select-Object LocalAddress,LocalPort,RemoteAddress,RemotePort,State,OwningProcess | ConvertTo-Json -Compress", "tcp")
	flows = append(flows, tcpFlows...)

	// Collect UDP endpoints
	udpFlows := collectPS("Get-NetUDPEndpoint | Select-Object LocalAddress,LocalPort,@{N='RemoteAddress';E={'0.0.0.0'}},@{N='RemotePort';E={0}},@{N='State';E={''}},OwningProcess | ConvertTo-Json -Compress", "udp")
	flows = append(flows, udpFlows...)

	// Resolve PIDs to process names
	pidMap := resolveProcessNames(flows)
	for i := range flows {
		if name, ok := pidMap[flows[i].PID]; ok {
			flows[i].Process = name
		}
	}

	return flows
}

func collectPS(script, proto string) []Flow {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	return parsePSConnections(out, proto)
}

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

// resolveProcessNames gets process names for all unique PIDs in one call
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
