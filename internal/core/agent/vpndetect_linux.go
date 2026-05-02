package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func detectVPNs() []VPNInfo {
	var vpns []VPNInfo

	// Method 1: Check for tun/tap interfaces
	entries, err := os.ReadDir("/sys/class/net")
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			// Check for VPN-like interface names
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

				// Try to read the interface type
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

	// Method 2: Check for VPN processes
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
