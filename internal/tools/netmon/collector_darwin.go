package netmon

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// collectPlatform uses lsof with machine-readable field output on macOS.
// lsof -i -n -P -F pcPtTn gives structured fields:
//
//	p<pid>  c<command>  P<protocol>  t<type>  T<state>  n<name (addr)>
func (c *Collector) collectPlatform() []Flow {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "lsof", "-i", "-n", "-P", "-F", "pcPtTn").Output()
	if err != nil {
		return nil
	}

	return parseLsofFields(string(out))
}

// parseLsofFields parses lsof -F output into flows.
// Each process block starts with p<pid>, then field lines.
// Network file entries have P<proto>, T<state>, n<addr> fields.
func parseLsofFields(output string) []Flow {
	var flows []Flow
	var pid int
	var process string
	var proto string
	var state string

	for _, line := range strings.Split(output, "\n") {
		if len(line) == 0 {
			continue
		}
		key := line[0]
		val := line[1:]

		switch key {
		case 'p':
			pid, _ = strconv.Atoi(val)
			process = ""
			proto = ""
			state = ""
		case 'c':
			process = val
		case 'P':
			proto = strings.ToLower(val)
		case 'T':
			// State field: "ST=ESTABLISHED" or just "ESTABLISHED"
			if strings.HasPrefix(val, "ST=") {
				state = val[3:]
			} else {
				state = val
			}
		case 'n':
			// Network name: "192.168.1.5:52341->142.250.80.46:443"
			// or "*:8082" (listening) or "localhost:53" (no arrow = local)
			if !strings.Contains(val, "->") {
				continue // listening or local-only socket
			}
			f := parseLsofAddr(val, proto)
			if f == nil {
				continue
			}
			f.PID = pid
			f.Process = process
			f.State = state
			flows = append(flows, *f)
			// Reset per-file fields
			state = ""
		}
	}

	return flows
}

// parseLsofAddr parses "local->remote" address pairs like "192.168.1.5:52341->142.250.80.46:443"
func parseLsofAddr(name, proto string) *Flow {
	parts := strings.SplitN(name, "->", 2)
	if len(parts) != 2 {
		return nil
	}

	localIP, localPort := splitHostPort(parts[0])
	remoteIP, remotePort := splitHostPort(parts[1])

	if remotePort == 0 {
		return nil
	}

	return &Flow{
		Proto:      proto,
		LocalIP:    localIP,
		LocalPort:  localPort,
		RemoteIP:   remoteIP,
		RemotePort: remotePort,
	}
}

// splitHostPort splits "addr:port" handling IPv6 bracket notation
func splitHostPort(s string) (string, int) {
	// Handle IPv6 [::1]:port
	if strings.HasPrefix(s, "[") {
		idx := strings.LastIndex(s, "]:")
		if idx < 0 {
			return s, 0
		}
		ip := s[1:idx]
		port, _ := strconv.Atoi(s[idx+2:])
		return ip, port
	}

	// IPv4 or hostname: last colon is port separator
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return s, 0
	}
	port, _ := strconv.Atoi(s[idx+1:])
	return s[:idx], port
}
