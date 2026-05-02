//go:build linux

package agent

import (
	"os"
)

// platformChecks returns the Linux-specific preflight checks: read
// access to /proc/net (required for connection collection) and a
// privilege probe used to warn when running unprivileged.
func platformChecks() []CheckResult {
	var results []CheckResult

	// /proc/net access — required for connection collection.
	results = append(results, checkProcNet())

	// Root or NET_ADMIN — needed to see PIDs for all sockets.
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
