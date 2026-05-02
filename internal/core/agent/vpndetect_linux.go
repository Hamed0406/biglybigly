package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// detectVPNs probes the Linux host for active VPN/proxy software.
// It combines two signals:
//
//  1. Interfaces under /sys/class/net whose name matches well-known
//     VPN prefixes (tun, tap, wg, proton, nordlynx, tailscale).
//  2. Running processes from a curated list of VPN daemons/clients.
//
// Process-based hits are deduplicated against interface-based hits so
// the same VPN isn't reported twice.
func detectVPNs() []VPNInfo {
	var vpns []VPNInfo

	// Method 1: VPN-style interfaces in /sys/class/net.
	entries, err := os.ReadDir("/sys/class/net")
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, "tun") ||
				strings.HasPrefix(name, "tap") ||
				strings.HasPrefix(name, "wg") ||
				strings.HasPrefix(name, "proton") ||
				strings.HasPrefix(name, "nordlynx") ||
				strings.HasPrefix(name, "tailscale") {

				v := VPNInfo{
					Name:      name,
					Interface: name,
				}

				// type == 65534 is ARPHRD_NONE — used by TUN devices —
				// so we annotate the name when we see it.
				typeFile := filepath.Join("/sys/class/net", name, "type")
				if data, err := os.ReadFile(typeFile); err == nil {
					t := strings.TrimSpace(string(data))
					if t == "65534" {
						v.Name = name + " (TUN)"
					}
				}

				vpns = append(vpns, v)
			}
		}
	}

	// Method 2: running VPN client processes.
	vpnProcesses := []string{
		"openvpn", "wireguard", "wg-quick",
		"nordvpnd", "expressvpn", "surfshark",
		"protonvpn", "cloudflare-warp", "warp-svc",
		"forticlient", "openconnect", "vpnc",
		"tailscaled",
	}

	for _, proc := range vpnProcesses {
		out, err := exec.Command("pgrep", "-x", proc).Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			found := false
			for _, v := range vpns {
				if strings.Contains(strings.ToLower(v.Name), proc) {
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
