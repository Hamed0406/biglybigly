package agent

import (
	"fmt"
	"os/exec"
	"strings"
)

// getActiveDNS picks the adapter carrying the lowest-metric default
// route (i.e. the host's primary egress interface) via Get-NetRoute and
// Get-NetAdapter, then reads its current IPv4 DNS servers with
// Get-DnsClientServerAddress.
func (d *DNSConfigurator) getActiveDNS() (iface string, dns []string, err error) {
	out, err := exec.Command("powershell", "-NoProfile", "-Command", `
		$route = Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object -Property RouteMetric | Select-Object -First 1
		if ($route) {
			$adapter = Get-NetAdapter -InterfaceIndex $route.InterfaceIndex
			$adapter.Name
		}
	`).Output()
	if err != nil {
		return "", nil, fmt.Errorf("detect adapter: %w", err)
	}

	iface = strings.TrimSpace(string(out))
	if iface == "" {
		return "", nil, fmt.Errorf("no active network adapter found")
	}

	out, err = exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`(Get-DnsClientServerAddress -InterfaceAlias '%s' -AddressFamily IPv4).ServerAddresses -join ','`, iface),
	).Output()
	if err != nil {
		// Adapter known but DNS unreadable — caller can still call
		// resetDNS to put the adapter back to DHCP on shutdown.
		return iface, nil, nil
	}

	dnsStr := strings.TrimSpace(string(out))
	if dnsStr != "" {
		dns = strings.Split(dnsStr, ",")
	}

	return iface, dns, nil
}

// setDNS applies a static list of IPv4 resolvers to the given adapter
// via Set-DnsClientServerAddress.
func (d *DNSConfigurator) setDNS(iface string, servers []string) error {
	serverList := strings.Join(servers, "','")
	cmd := fmt.Sprintf(`Set-DnsClientServerAddress -InterfaceAlias '%s' -ServerAddresses @('%s')`, iface, serverList)

	out, err := exec.Command("powershell", "-NoProfile", "-Command", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("set DNS: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resetDNS reverts the adapter to DHCP-supplied DNS servers using
// the -ResetServerAddresses flag.
func (d *DNSConfigurator) resetDNS(iface string) error {
	cmd := fmt.Sprintf(`Set-DnsClientServerAddress -InterfaceAlias '%s' -ResetServerAddresses`, iface)

	out, err := exec.Command("powershell", "-NoProfile", "-Command", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("reset DNS: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
