package agent

import (
	"log/slog"
	"runtime"
)

// VPNInfo holds detected VPN/proxy information
type VPNInfo struct {
	Name      string
	Interface string
	DNS       []string
}

// DetectVPN checks for active VPN or proxy connections and logs guidance.
// Returns true if a VPN was detected.
func DetectVPN(logger *slog.Logger) bool {
	vpns := detectVPNs()
	if len(vpns) == 0 {
		logger.Info("  ✓ No VPN/proxy detected")
		return false
	}

	logger.Warn("══════════════════════════════════════════════════")
	logger.Warn("  ⚠  VPN/Proxy Detected")
	logger.Warn("──────────────────────────────────────────────────")

	for _, v := range vpns {
		logger.Warn("  VPN found", "name", v.Name, "interface", v.Interface)
		if len(v.DNS) > 0 {
			logger.Warn("  VPN DNS servers", "dns", v.DNS)
		}
	}

	logger.Warn("")
	logger.Warn("  VPNs often override DNS settings, which prevents")
	logger.Warn("  the DNS filter from intercepting queries.")
	logger.Warn("")
	logger.Warn("  Options to fix:")

	switch runtime.GOOS {
	case "windows":
		logger.Warn("  1. Set VPN client DNS to 127.0.0.1 in VPN settings")
		logger.Warn("  2. WireGuard: edit tunnel, set DNS = 127.0.0.1")
		logger.Warn("  3. OpenVPN: add 'dhcp-option DNS 127.0.0.1' to .ovpn")
		logger.Warn("  4. Disconnect VPN while using DNS filter")
	case "darwin":
		logger.Warn("  1. Set VPN client DNS to 127.0.0.1 in VPN settings")
		logger.Warn("  2. WireGuard: edit tunnel, set DNS = 127.0.0.1")
		logger.Warn("  3. OpenVPN: add 'dhcp-option DNS 127.0.0.1' to config")
		logger.Warn("  4. Disconnect VPN while using DNS filter")
	default:
		logger.Warn("  1. Set VPN DNS to 127.0.0.1 in VPN config")
		logger.Warn("  2. WireGuard: set DNS = 127.0.0.1 in [Interface]")
		logger.Warn("  3. OpenVPN: add 'dhcp-option DNS 127.0.0.1' to config")
		logger.Warn("  4. Disconnect VPN while using DNS filter")
	}

	logger.Warn("══════════════════════════════════════════════════")
	return true
}
