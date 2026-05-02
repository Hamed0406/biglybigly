//go:build windows

package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// platformChecks returns the Windows-specific preflight checks:
// availability of PowerShell (used by the collector and DNS configurator),
// presence of the Npcap driver for packet capture, and a probe for
// elevated/admin privileges.
func platformChecks() []CheckResult {
	var results []CheckResult

	// PowerShell — required for Get-NetTCPConnection and DNS config.
	results = append(results, checkPowerShell())

	// Npcap driver — needed for packet capture.
	results = append(results, checkNpcap())

	// Admin / elevated privileges — needed for full connection info.
	results = append(results, checkElevated())

	return results
}

func checkPowerShell() CheckResult {
	path, err := exec.LookPath("powershell")
	if err != nil {
		return CheckResult{
			Name:     "PowerShell",
			Status:   "fail",
			Detail:   "not found in PATH — required for network collection",
			Required: true,
		}
	}
	return CheckResult{
		Name:     "PowerShell",
		Status:   "ok",
		Detail:   path,
		Required: true,
	}
}

func checkNpcap() CheckResult {
	// Npcap may be reported either by its installed wpcap.dll or by
	// the running service; try both before declaring it missing.
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}

	npcapDLL := filepath.Join(systemRoot, "System32", "Npcap", "wpcap.dll")
	if _, err := os.Stat(npcapDLL); err == nil {
		return CheckResult{
			Name:     "Npcap Driver",
			Status:   "ok",
			Detail:   "installed at " + npcapDLL,
			Required: false,
		}
	}

	// Also check via sc query in case the DLL location differs.
	out, err := exec.Command("sc", "query", "npcap").Output()
	if err == nil && strings.Contains(string(out), "RUNNING") {
		return CheckResult{
			Name:     "Npcap Driver",
			Status:   "ok",
			Detail:   "service running",
			Required: false,
		}
	}

	return CheckResult{
		Name:     "Npcap Driver",
		Status:   "warn",
		Detail:   "not found — install from https://npcap.com for packet capture",
		Required: false,
	}
}

func checkElevated() CheckResult {
	// Opening a raw physical drive succeeds only for Administrators;
	// it is a cheap, dependency-free elevation probe on Windows.
	_, err := os.Open(`\\.\PHYSICALDRIVE0`)
	if err != nil {
		return CheckResult{
			Name:     "Admin Privileges",
			Status:   "warn",
			Detail:   "not elevated — some connections may be missing process info",
			Required: false,
		}
	}
	return CheckResult{
		Name:     "Admin Privileges",
		Status:   "ok",
		Detail:   "running elevated",
		Required: false,
	}
}
