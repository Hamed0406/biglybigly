package agent

import (
	"os/exec"
	"strings"
)

// detectVPNs probes the Windows host for active VPN/proxy software.
// It runs two PowerShell pipelines:
//
//  1. Get-NetAdapter is filtered for adapters whose description or name
//     matches well-known VPN/proxy keywords (TAP/TUN, WireGuard,
//     OpenVPN, Cisco AnyConnect, Fortinet, Pulse, GlobalProtect,
//     ProtonVPN.Client, Cloudflare WARP, etc.). For each match the
//     IPv4 DNS servers configured on the adapter are also captured.
//  2. Get-Process is filtered for a curated list of VPN client process
//     names; matches not already covered by an adapter hit are added
//     so background VPN services without a visible adapter are still
//     reported.
func detectVPNs() []VPNInfo {
	var vpns []VPNInfo

	// Method 1: VPN-flavoured network adapters that are currently up.
	out, err := exec.Command("powershell", "-NoProfile", "-Command", `
		Get-NetAdapter | Where-Object {
			$_.Status -eq 'Up' -and (
				$_.InterfaceDescription -match 'VPN|TAP|TUN|WireGuard|OpenVPN|Cisco|Fortinet|Pulse|GlobalProtect|NordVPN|ExpressVPN|Surfshark|ProtonVPN|Cloudflare|WARP' -or
				$_.Name -match 'VPN|TAP|TUN|WireGuard|wg|tun|Proton|Nord'
			)
		} | ForEach-Object {
			$dns = (Get-DnsClientServerAddress -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue).ServerAddresses -join ','
			"$($_.Name)|$($_.InterfaceDescription)|$dns"
		}
	`).Output()

	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 3)
			if len(parts) < 2 {
				continue
			}

			v := VPNInfo{
				Name:      parts[1],
				Interface: parts[0],
			}
			if len(parts) >= 3 && parts[2] != "" {
				v.DNS = strings.Split(parts[2], ",")
			}
			vpns = append(vpns, v)
		}
	}

	// Method 2: running VPN client processes (catches services with no
	// visible adapter, e.g. background helpers).
	processOut, err := exec.Command("powershell", "-NoProfile", "-Command", `
		$vpnProcesses = @(
			'openvpn', 'wireguard', 'vpnui', 'vpncli',
			'nordvpn', 'expressvpn', 'surfshark',
			'protonvpn', 'cloudflare-warp', 'warp-svc',
			'FortiClient', 'dsAccessService', 'PanGPA',
			'TailscaleIPN', 'tailscaled'
		)
		Get-Process -ErrorAction SilentlyContinue | Where-Object {
			$name = $_.ProcessName.ToLower()
			$vpnProcesses | Where-Object { $name -like "*$_*" }
		} | Select-Object -Unique ProcessName | ForEach-Object { $_.ProcessName }
	`).Output()

	if err == nil {
		for _, proc := range strings.Split(strings.TrimSpace(string(processOut)), "\n") {
			proc = strings.TrimSpace(proc)
			if proc == "" {
				continue
			}
			// Skip processes whose name was already surfaced as an
			// adapter match so we don't double-report the same VPN.
			found := false
			for _, v := range vpns {
				if strings.EqualFold(v.Name, proc) || strings.Contains(strings.ToLower(v.Name), strings.ToLower(proc)) {
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
