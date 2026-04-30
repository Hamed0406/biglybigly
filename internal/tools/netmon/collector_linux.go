package netmon

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// collectPlatform reads /proc/net/tcp, tcp6, udp, udp6
func (c *Collector) collectPlatform() []Flow {
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
		flows = append(flows, parseProcNet(entry.path, entry.proto)...)
	}

	// Enrich with process names from /proc
	for i := range flows {
		if flows[i].PID > 0 {
			flows[i].Process = getProcessName(flows[i].PID)
		}
	}

	return flows
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

		localIP, localPort := ParseHexAddr(fields[1])
		remoteIP, remotePort := ParseHexAddr(fields[2])
		state := parseState(fields[3])

		var pid int
		if len(fields) >= 10 {
			pid = findPIDByInode(fields[9])
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

// ParseHexAddr converts "0100007F:0035" to ("127.0.0.1", 53)
func ParseHexAddr(s string) (string, int) {
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
		ip := net.IPv4(b[3], b[2], b[1], b[0])
		return ip.String(), int(port)
	case 32: // IPv6 — stored as 4 groups of 4 bytes, each group little-endian
		b, err := hex.DecodeString(hexIP)
		if err != nil || len(b) != 16 {
			return "", int(port)
		}
		for i := 0; i < 16; i += 4 {
			b[i], b[i+3] = b[i+3], b[i]
			b[i+1], b[i+2] = b[i+2], b[i+1]
		}
		ip := net.IP(b)
		return ip.String(), int(port)
	}
	return "", int(port)
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
