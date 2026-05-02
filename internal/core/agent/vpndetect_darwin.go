package agent

import (
	"os/exec"
	"strings"
)

// detectVPNs probes the macOS host for active VPN/proxy software using
// three complementary signals:
//
//  1. utun/tun/tap interfaces returned by `ifconfig -l` (utun is the
//     standard kernel TUN device used by all modern macOS VPN clients).
//  2. Connected entries from `scutil --nc list` (system VPN services).
//  3. Running processes from a curated list of VPN client names.
//
// Hits from later methods are deduplicated against earlier ones.
func detectVPNs() []VPNInfo {
	var vpns []VPNInfo

	// Method 1: utun-style interfaces.
	out, err := exec.Command("ifconfig", "-l").Output()
	if err == nil {
		for _, iface := range strings.Fields(string(out)) {
			if strings.HasPrefix(iface, "utun") ||
				strings.HasPrefix(iface, "tun") ||
				strings.HasPrefix(iface, "tap") {
				vpns = append(vpns, VPNInfo{
					Name:      iface,
					Interface: iface,
				})
			}
		}
	}

	// Method 2: connected network services per scutil.
	out, err = exec.Command("scutil", "--nc", "list").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "Connected") {
				// scutil prints the service name in quotes; extract
				// whatever lies between the first and last quote.
				start := strings.Index(line, `"`)
				end := strings.LastIndex(line, `"`)
				if start >= 0 && end > start {
					name := line[start+1 : end]
					vpns = append(vpns, VPNInfo{
						Name:      name + " (Network Service)",
						Interface: "scutil",
					})
				}
			}
		}
	}

	// Method 3: running VPN processes.
	vpnProcesses := []string{
		"openvpn", "wireguard-go",
		"NordVPN", "ExpressVPN", "Surfshark",
		"ProtonVPN", "CloudflareWARP",
		"Tailscale", "tailscaled",
	}

	for _, proc := range vpnProcesses {
		out, err := exec.Command("pgrep", "-i", proc).Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			found := false
			for _, v := range vpns {
				if strings.Contains(strings.ToLower(v.Name), strings.ToLower(proc)) {
					found = true
					break
				}
			}
			if !found {
				vpns = append(vpns, VPNInfo{
					Name:      proc + " (process)",
					Interface: "detected via process list",
				})
			}
		}
	}

	return vpns
}
