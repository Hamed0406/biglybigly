package agent

import (
	"log/slog"
	"runtime"
)

// DNSConfigurator manages system DNS settings for the DNS proxy.
// It saves the original DNS on start and restores it on shutdown.
type DNSConfigurator struct {
	logger      *slog.Logger
	originalDNS []string
	iface       string
	configured  bool
}

func NewDNSConfigurator(logger *slog.Logger) *DNSConfigurator {
	return &DNSConfigurator{logger: logger}
}

// Configure sets the system DNS to 127.0.0.1 for the active network adapter.
// Returns true if DNS was changed successfully.
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

// Restore reverts DNS to the original settings. Must be called on shutdown.
func (d *DNSConfigurator) Restore() {
	if !d.configured {
		return
	}

	d.logger.Info("DNS auto-config: restoring original DNS",
		"interface", d.iface,
		"dns", d.originalDNS,
	)

	if len(d.originalDNS) == 0 {
		// Restore to DHCP/automatic
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
