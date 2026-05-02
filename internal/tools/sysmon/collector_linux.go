package sysmon

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// collectCPU reads /proc/stat and returns the CPU busy percentage relative
// to prev. The first call (prev == nil) returns 0 along with the new sample.
func collectCPU(prev *cpuSample) (float64, *cpuSample) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, prev
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0, prev
	}
	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") {
		return 0, prev
	}

	fields := strings.Fields(line)
	if len(fields) < 5 {
		return 0, prev
	}

	var total, idle uint64
	for i, field := range fields[1:] {
		val, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			continue
		}
		total += val
		if i == 3 { // 4th field is idle
			idle = val
		}
	}

	cur := &cpuSample{idle: idle, total: total}
	if prev == nil {
		return 0, cur
	}

	deltaTotal := float64(cur.total - prev.total)
	deltaIdle := float64(cur.idle - prev.idle)
	if deltaTotal == 0 {
		return 0, cur
	}

	cpuPercent := (1.0 - deltaIdle/deltaTotal) * 100.0
	return cpuPercent, cur
}

// collectMemory parses /proc/meminfo. "used" is derived as MemTotal -
// MemAvailable, which matches what `free` reports.
func collectMemory() (total, used, available uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, err
	}
	defer f.Close()

	values := make(map[string]uint64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		val, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}
		values[key] = val * 1024 // convert kB to bytes
	}

	total = values["MemTotal"]
	available = values["MemAvailable"]
	used = total - available

	return total, used, available, nil
}

// collectLoadAvg reads the 1/5/15 minute load averages from /proc/loadavg.
func collectLoadAvg() (l1, l5, l15 float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		l1, _ = strconv.ParseFloat(fields[0], 64)
		l5, _ = strconv.ParseFloat(fields[1], 64)
		l15, _ = strconv.ParseFloat(fields[2], 64)
	}
	return
}

// collectOSInfo returns PRETTY_NAME from /etc/os-release, falling back to
// "Linux".
func collectOSInfo() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			name := strings.TrimPrefix(line, "PRETTY_NAME=")
			name = strings.Trim(name, `"`)
			return name
		}
	}
	return "Linux"
}

// collectHostname returns os.Hostname(), or "unknown" on error.
func collectHostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}

// collectUptime reads the first field of /proc/uptime (seconds since boot).
func collectUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(secs)
}

// collectDisks runs df with byte-sized output, excluding pseudo filesystems.
// Falls back to plain df parsing when the GNU-specific flags are unsupported.
func collectDisks() ([]DiskInfo, error) {
	out, err := exec.Command("df", "-B1", "--output=target,fstype,size,used,avail", "--exclude-type=tmpfs", "--exclude-type=devtmpfs", "--exclude-type=squashfs", "--exclude-type=overlay").Output()
	if err != nil {
		// Fallback: try simpler df
		return collectDisksFallback()
	}
	return parseDfOutput(string(out)), nil
}

func collectDisksFallback() ([]DiskInfo, error) {
	out, err := exec.Command("df", "-B1").Output()
	if err != nil {
		return nil, fmt.Errorf("df failed: %w", err)
	}
	return parseDfSimple(string(out)), nil
}

func parseDfOutput(output string) []DiskInfo {
	var disks []DiskInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i, line := range lines {
		if i == 0 { // skip header
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		total, _ := strconv.ParseUint(fields[2], 10, 64)
		used, _ := strconv.ParseUint(fields[3], 10, 64)
		avail, _ := strconv.ParseUint(fields[4], 10, 64)
		disks = append(disks, DiskInfo{
			MountPoint: fields[0],
			FSType:     fields[1],
			TotalBytes: total,
			UsedBytes:  used,
			AvailBytes: avail,
		})
	}
	return disks
}

func parseDfSimple(output string) []DiskInfo {
	var disks []DiskInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		// Skip pseudo-filesystems
		if strings.HasPrefix(fields[0], "tmpfs") || strings.HasPrefix(fields[0], "devtmpfs") || fields[0] == "none" {
			continue
		}
		total, _ := strconv.ParseUint(fields[1], 10, 64)
		used, _ := strconv.ParseUint(fields[2], 10, 64)
		avail, _ := strconv.ParseUint(fields[3], 10, 64)
		mount := fields[5]
		disks = append(disks, DiskInfo{
			MountPoint: mount,
			TotalBytes: total,
			UsedBytes:  used,
			AvailBytes: avail,
		})
	}
	return disks
}

// formatUptime returns human-readable uptime
func formatUptime(_ time.Duration) string {
	return ""
}
