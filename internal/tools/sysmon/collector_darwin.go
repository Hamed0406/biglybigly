package sysmon

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// collectCPU reads kern.cp_time via sysctl and computes the busy percentage
// relative to the previous sample. macOS lays out the tick counters as
// user/nice/sys/intr/idle, so the 5th field is the idle counter.
func collectCPU(prev *cpuSample) (float64, *cpuSample) {
	out, err := exec.Command("sysctl", "-n", "kern.cp_time").Output()
	if err != nil {
		return 0, prev
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 5 {
		return 0, prev
	}

	var total, idle uint64
	for i, field := range fields {
		val, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			continue
		}
		total += val
		if i == 4 { // 5th field is idle on macOS
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

// collectMemory queries hw.memsize, hw.pagesize and vm_stat. "available" is
// approximated as (free + speculative + inactive) pages * pagesize, which
// matches what Activity Monitor reports as available memory.
func collectMemory() (total, used, available uint64, err error) {
	// Total memory via sysctl
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("sysctl memsize: %w", err)
	}
	total, _ = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)

	// Get page size
	pageSizeOut, err := exec.Command("sysctl", "-n", "hw.pagesize").Output()
	if err != nil {
		return total, 0, 0, nil
	}
	pageSize, _ := strconv.ParseUint(strings.TrimSpace(string(pageSizeOut)), 10, 64)

	// Use vm_stat for free/inactive pages
	vmOut, err := exec.Command("vm_stat").Output()
	if err != nil {
		return total, 0, 0, nil
	}

	pages := make(map[string]uint64)
	for _, line := range strings.Split(string(vmOut), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, ".")
		val, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}
		pages[key] = val
	}

	freePages := pages["Pages free"] + pages["Pages speculative"]
	inactivePages := pages["Pages inactive"]
	available = (freePages + inactivePages) * pageSize
	used = total - available

	return total, used, available, nil
}

// collectLoadAvg parses vm.loadavg ("{ 1.23 4.56 7.89 }").
func collectLoadAvg() (l1, l5, l15 float64) {
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0, 0, 0
	}
	// Output: "{ 1.23 4.56 7.89 }"
	s := strings.Trim(strings.TrimSpace(string(out)), "{}")
	fields := strings.Fields(s)
	if len(fields) >= 3 {
		l1, _ = strconv.ParseFloat(fields[0], 64)
		l5, _ = strconv.ParseFloat(fields[1], 64)
		l15, _ = strconv.ParseFloat(fields[2], 64)
	}
	return
}

// collectOSInfo returns "macOS <version>" via sw_vers.
func collectOSInfo() string {
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return "macOS"
	}
	return "macOS " + strings.TrimSpace(string(out))
}

// collectHostname returns os.Hostname(), or "unknown" on error.
func collectHostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}

// collectUptime parses kern.boottime ("{ sec = N, usec = … }") and returns
// seconds since boot.
func collectUptime() int64 {
	out, err := exec.Command("sysctl", "-n", "kern.boottime").Output()
	if err != nil {
		return 0
	}
	// Output: "{ sec = 1234567890, usec = 123456 } ..."
	s := string(out)
	idx := strings.Index(s, "sec = ")
	if idx < 0 {
		return 0
	}
	s = s[idx+6:]
	endIdx := strings.Index(s, ",")
	if endIdx < 0 {
		return 0
	}
	bootSec, err := strconv.ParseInt(s[:endIdx], 10, 64)
	if err != nil {
		return 0
	}
	return time.Now().Unix() - bootSec
}

// collectDisks parses `df -b` output. macOS df reports 512-byte blocks, so
// values are scaled to bytes here.
func collectDisks() ([]DiskInfo, error) {
	out, err := exec.Command("df", "-b").Output()
	if err != nil {
		return nil, fmt.Errorf("df failed: %w", err)
	}

	var disks []DiskInfo
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		// Skip non-physical filesystems
		if strings.HasPrefix(fields[0], "devfs") || fields[0] == "map" || strings.HasPrefix(fields[0], "map ") {
			continue
		}
		total, _ := strconv.ParseUint(fields[1], 10, 64)
		used, _ := strconv.ParseUint(fields[2], 10, 64)
		avail, _ := strconv.ParseUint(fields[3], 10, 64)

		// df -b on macOS reports 512-byte blocks
		total *= 512
		used *= 512
		avail *= 512

		mount := fields[8]
		disks = append(disks, DiskInfo{
			MountPoint: mount,
			TotalBytes: total,
			UsedBytes:  used,
			AvailBytes: avail,
		})
	}
	return disks, nil
}
