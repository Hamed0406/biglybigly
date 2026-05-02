package agent

import (
	"fmt"
	"os/exec"
	"strings"
)

func (d *DNSConfigurator) getActiveDNS() (iface string, dns []string, err error) {
	// Get the primary network service
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return "", nil, fmt.Errorf("list services: %w", err)
	}

	// Find first active service (skip the header line)
	var service string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "*") || strings.Contains(line, "asterisk") {
			continue
		}
		// Check if this service has an IP (is active)
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

	// Get current DNS
	out, err = exec.Command("networksetup", "-getdnsservers", service).Output()
	if err != nil {
		return service, nil, nil
	}

	dnsStr := strings.TrimSpace(string(out))
	if strings.Contains(dnsStr, "aren't any") {
		return service, nil, nil // Using DHCP DNS
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

func (d *DNSConfigurator) setDNS(iface string, servers []string) error {
	args := append([]string{"-setdnsservers", iface}, servers...)
	out, err := exec.Command("networksetup", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("set DNS: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (d *DNSConfigurator) resetDNS(iface string) error {
	out, err := exec.Command("networksetup", "-setdnsservers", iface, "Empty").CombinedOutput()
	if err != nil {
		return fmt.Errorf("reset DNS: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
