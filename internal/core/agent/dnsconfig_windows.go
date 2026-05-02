package agent

import (
	"fmt"
	"os/exec"
	"strings"
)

func (d *DNSConfigurator) getActiveDNS() (iface string, dns []string, err error) {
	// Get the active interface with default route
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

	// Get current DNS servers
	out, err = exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`(Get-DnsClientServerAddress -InterfaceAlias '%s' -AddressFamily IPv4).ServerAddresses -join ','`, iface),
	).Output()
	if err != nil {
		return iface, nil, nil // Can't get DNS but we know the interface
	}

	dnsStr := strings.TrimSpace(string(out))
	if dnsStr != "" {
		dns = strings.Split(dnsStr, ",")
	}

	return iface, dns, nil
}

func (d *DNSConfigurator) setDNS(iface string, servers []string) error {
	serverList := strings.Join(servers, "','")
	cmd := fmt.Sprintf(`Set-DnsClientServerAddress -InterfaceAlias '%s' -ServerAddresses @('%s')`, iface, serverList)

	out, err := exec.Command("powershell", "-NoProfile", "-Command", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("set DNS: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (d *DNSConfigurator) resetDNS(iface string) error {
	cmd := fmt.Sprintf(`Set-DnsClientServerAddress -InterfaceAlias '%s' -ResetServerAddresses`, iface)

	out, err := exec.Command("powershell", "-NoProfile", "-Command", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("reset DNS: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
