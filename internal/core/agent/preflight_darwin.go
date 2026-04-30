//go:build darwin

package agent

import (
	"os"
	"os/exec"
)

func platformChecks() []CheckResult {
	var results []CheckResult

	// 1. lsof — required for connection collection on macOS
	results = append(results, checkLsof())

	// 2. Root — for full PID/process info
	results = append(results, checkPrivilegesDarwin())

	return results
}

func checkLsof() CheckResult {
	path, err := exec.LookPath("lsof")
	if err != nil {
		return CheckResult{
			Name:     "lsof",
			Status:   "fail",
			Detail:   "not found in PATH — required for network collection",
			Required: true,
		}
	}
	return CheckResult{
		Name:     "lsof",
		Status:   "ok",
		Detail:   path,
		Required: true,
	}
}

func checkPrivilegesDarwin() CheckResult {
	if os.Getuid() == 0 {
		return CheckResult{
			Name:     "Privileges",
			Status:   "ok",
			Detail:   "running as root",
			Required: false,
		}
	}
	return CheckResult{
		Name:     "Privileges",
		Status:   "warn",
		Detail:   "not root — some process info may be unavailable",
		Required: false,
	}
}
