package agent

import (
	"fmt"
	"os/exec"
	"strings"
)

// getActiveDNS finds the first network service with an assigned IPv4
// address (using `networksetup -listallnetworkservices` and
// `-getinfo`) and reads its currently configured DNS servers via
// `networksetup -getdnsservers`. A service with DHCP-managed DNS
// reports "aren't any" and is returned as an empty list.
func (d *DNSConfigurator) getActiveDNS() (iface string, dns []string, err error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return "", nil, fmt.Errorf("list services: %w", err)
	}

	// Walk services in priority order; the header line and any
	// service prefixed with "*" (disabled) are skipped.
	var service string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "*") || strings.Contains(line, "asterisk") {
			continue
		}
		// Treat presence of "IP address:" as a proxy for "active".
		ipOut, err := exec.Command("networksetup", "-getinfo", line).Output()
		if err != nil {
			continue
		}
		if strings.Contains(string(ipOut), "IP address:") {
			service = line
			break
		}
	}

	if service == "" {
		return "", nil, fmt.Errorf("no active network service found")
	}

	out, err = exec.Command("networksetup", "-getdnsservers", service).Output()
	if err != nil {
		return service, nil, nil
	}

	dnsStr := strings.TrimSpace(string(out))
	if strings.Contains(dnsStr, "aren't any") {
		// "There aren't any DNS Servers set on <service>." — DHCP DNS.
		return service, nil, nil
	}

	var servers []string
	for _, line := range strings.Split(dnsStr, "\n") {
		s := strings.TrimSpace(line)
		if s != "" {
			servers = append(servers, s)
		}
	}

	return service, servers, nil
}

// setDNS applies a static list of resolvers to the given network
// service via `networksetup -setdnsservers`.
func (d *DNSConfigurator) setDNS(iface string, servers []string) error {
	args := append([]string{"-setdnsservers", iface}, servers...)
	out, err := exec.Command("networksetup", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("set DNS: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resetDNS clears any static DNS servers from the service so it falls
// back to DHCP-supplied resolvers ("Empty" is networksetup's sentinel).
func (d *DNSConfigurator) resetDNS(iface string) error {
	out, err := exec.Command("networksetup", "-setdnsservers", iface, "Empty").CombinedOutput()
	if err != nil {
		return fmt.Errorf("reset DNS: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
