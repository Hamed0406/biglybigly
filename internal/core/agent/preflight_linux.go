//go:build linux

package agent

import (
	"os"
)

func platformChecks() []CheckResult {
	var results []CheckResult

	// 1. /proc/net access — required for connection collection
	results = append(results, checkProcNet())

	// 2. Root or NET_ADMIN — for full PID info
	results = append(results, checkPrivileges())

	return results
}

func checkProcNet() CheckResult {
	if _, err := os.Stat("/proc/net/tcp"); err != nil {
		return CheckResult{
			Name:     "/proc/net",
			Status:   "fail",
			Detail:   "/proc/net/tcp not accessible — required for network collection",
			Required: true,
		}
	}
	return CheckResult{
		Name:     "/proc/net",
		Status:   "ok",
		Detail:   "accessible",
		Required: true,
	}
}

func checkPrivileges() CheckResult {
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
