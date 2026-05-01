package sysmon

import (
	"log/slog"
	"os"
	"testing"
)

func TestCollect(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c := NewCollector(logger)

	snap, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect() error: %v", err)
	}

	if snap.CollectedAt == 0 {
		t.Error("CollectedAt should be set")
	}
	if snap.Hostname == "" {
		t.Error("Hostname should be set")
	}

	// Second collect should give CPU delta
	snap2, err := c.Collect()
	if err != nil {
		t.Fatalf("Second Collect() error: %v", err)
	}
	// CPU may still be 0 on very fast machines, but shouldn't error
	_ = snap2
}

func TestCollectMemory(t *testing.T) {
	total, used, avail, err := collectMemory()
	if err != nil {
		t.Skipf("Memory collection not available: %v", err)
	}

	if total == 0 {
		t.Error("Total memory should be > 0")
	}
	if used == 0 {
		t.Error("Used memory should be > 0")
	}
	if avail == 0 {
		t.Error("Available memory should be > 0")
	}
	if used+avail > total*2 {
		t.Errorf("used (%d) + avail (%d) is way more than total (%d)", used, avail, total)
	}
}

func TestCollectUptime(t *testing.T) {
	secs := collectUptime()
	if secs <= 0 {
		t.Skip("Uptime not available")
	}
}

func TestCollectHostname(t *testing.T) {
	name := collectHostname()
	if name == "" || name == "unknown" {
		t.Skip("Hostname not available")
	}
}
