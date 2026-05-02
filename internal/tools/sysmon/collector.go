package sysmon

import (
	"log/slog"
	"sync"
	"time"
)

// SystemSnapshot is a point-in-time measurement of one host. It mirrors a
// row in sysmon_snapshots plus the associated disk rows.
type SystemSnapshot struct {
	CPUPercent   float64    `json:"cpu_percent"`
	MemTotal     uint64     `json:"mem_total"`
	MemUsed      uint64     `json:"mem_used"`
	MemAvailable uint64     `json:"mem_available"`
	Load1        float64    `json:"load1"`
	Load5        float64    `json:"load5"`
	Load15       float64    `json:"load15"`
	OSInfo       string     `json:"os_info"`
	Hostname     string     `json:"hostname"`
	UptimeSecs   int64      `json:"uptime_secs"`
	Disks        []DiskInfo `json:"disks"`
	CollectedAt  int64      `json:"collected_at"`
}

// DiskInfo describes usage for one mounted filesystem.
type DiskInfo struct {
	MountPoint string `json:"mount_point"`
	FSType     string `json:"fs_type"`
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
	AvailBytes uint64 `json:"avail_bytes"`
}

// Collector gathers system metrics. CPU usage requires two samples to
// compute, so the previous sample is retained between calls under mu.
type Collector struct {
	logger   *slog.Logger
	mu       sync.Mutex
	prevCPU  *cpuSample
	prevTime time.Time
}

// cpuSample is a raw CPU tick snapshot used to compute a percentage by delta.
type cpuSample struct {
	idle  uint64
	total uint64
}

// NewCollector returns a Collector ready for repeated Collect calls.
func NewCollector(logger *slog.Logger) *Collector {
	return &Collector{logger: logger}
}

// Collect gathers a full SystemSnapshot using platform-specific helpers.
// The first call returns 0 for CPUPercent because there is no prior sample
// to delta against.
func (c *Collector) Collect() (*SystemSnapshot, error) {
	snap := &SystemSnapshot{
		CollectedAt: time.Now().Unix(),
	}

	// CPU (uses delta from previous sample)
	c.mu.Lock()
	cpuPercent, newSample := collectCPU(c.prevCPU)
	c.prevCPU = newSample
	c.mu.Unlock()
	snap.CPUPercent = cpuPercent

	// Memory
	memTotal, memUsed, memAvail, err := collectMemory()
	if err != nil {
		c.logger.Warn("sysmon: memory collection failed", "err", err)
	} else {
		snap.MemTotal = memTotal
		snap.MemUsed = memUsed
		snap.MemAvailable = memAvail
	}

	// Load average
	l1, l5, l15 := collectLoadAvg()
	snap.Load1 = l1
	snap.Load5 = l5
	snap.Load15 = l15

	// OS info
	snap.OSInfo = collectOSInfo()
	snap.Hostname = collectHostname()
	snap.UptimeSecs = collectUptime()

	// Disks
	disks, err := collectDisks()
	if err != nil {
		c.logger.Warn("sysmon: disk collection failed", "err", err)
	} else {
		snap.Disks = disks
	}

	return snap, nil
}
