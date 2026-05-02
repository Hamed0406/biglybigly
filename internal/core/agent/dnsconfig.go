package agent

import (
	"log/slog"
	"runtime"
)

// DNSConfigurator manages the host's system DNS settings on behalf of
// the local DNS filter. On Configure it records the currently configured
// upstream resolvers and points the active network adapter at 127.0.0.1
// so that all queries flow through the agent. Restore must be called on
// shutdown (typically via defer) to put the original resolvers back; if
// it is missed the host will be left pointing at a no-longer-running
// loopback resolver.
//
// The platform-specific helpers (getActiveDNS, setDNS, resetDNS) live in
// dnsconfig_{linux,darwin,windows}.go.
type DNSConfigurator struct {
	logger      *slog.Logger
	originalDNS []string
	iface       string
	configured  bool
}

// NewDNSConfigurator returns a DNSConfigurator that has not yet touched
// the system. Call Configure to apply, and defer Restore to roll back.
func NewDNSConfigurator(logger *slog.Logger) *DNSConfigurator {
	return &DNSConfigurator{logger: logger}
}

// Configure detects the active network adapter, records its current DNS
// servers, and sets it to use 127.0.0.1. It returns true on success;
// on failure it logs a warning and leaves the system untouched so the
// caller can continue running without auto-config.
func (d *DNSConfigurator) Configure() bool {
	iface, original, err := d.getActiveDNS()
	if err != nil {
		d.logger.Warn("DNS auto-config: failed to detect active adapter", "err", err)
		return false
	}

	d.iface = iface
	d.originalDNS = original
	d.logger.Info("DNS auto-config: detected adapter",
		"interface", iface,
		"original_dns", original,
		"os", runtime.GOOS,
	)

	if err := d.setDNS(iface, []string{"127.0.0.1"}); err != nil {
		d.logger.Warn("DNS auto-config: failed to set DNS", "err", err)
		return false
	}

	d.configured = true
	d.logger.Info("DNS auto-config: set DNS to 127.0.0.1",
		"interface", iface,
		"previous_dns", original,
	)
	return true
}

// Restore reverts DNS to whatever was in effect when Configure was
// called. It is a no-op if Configure was never called or already failed.
// If the original list was empty (DHCP-managed) it asks the platform to
// reset to automatic; otherwise it re-applies the recorded servers.
// Failures here are logged at error level because they leave the host
// without working DNS.
func (d *DNSConfigurator) Restore() {
	if !d.configured {
		return
	}

	d.logger.Info("DNS auto-config: restoring original DNS",
		"interface", d.iface,
		"dns", d.originalDNS,
	)

	if len(d.originalDNS) == 0 {
		// No explicit servers were set — restore to DHCP/automatic.
		if err := d.resetDNS(d.iface); err != nil {
			d.logger.Error("DNS auto-config: FAILED to restore DNS — set DNS manually!", "err", err)
			return
		}
	} else {
		if err := d.setDNS(d.iface, d.originalDNS); err != nil {
			d.logger.Error("DNS auto-config: FAILED to restore DNS — set DNS manually!",
				"err", err, "original_dns", d.originalDNS)
			return
		}
	}

	d.configured = false
	d.logger.Info("DNS auto-config: original DNS restored successfully")
}
