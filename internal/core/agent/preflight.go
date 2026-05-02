package agent

import (
	"fmt"
	"log/slog"
	"runtime"
)

// CheckResult represents the outcome of a single preflight check
// performed at agent startup.
type CheckResult struct {
	Name     string // e.g. "PowerShell", "Npcap Driver"
	Status   string // "ok", "warn", "fail"
	Detail   string // human-readable detail
	Required bool   // if true and failed, agent should not start
}

// Icon returns a single-character glyph (✓ / ⚠ / ✗) suitable for
// rendering the check status in log output.
func (r CheckResult) Icon() string {
	switch r.Status {
	case "ok":
		return "✓"
	case "warn":
		return "⚠"
	default:
		return "✗"
	}
}

// RunPreflight runs all platform-appropriate startup checks (provided by
// the per-OS platformChecks function) and writes a formatted summary to
// logger. It returns false if any required check failed, in which case
// the caller should refuse to start the agent.
func RunPreflight(logger *slog.Logger) bool {
	checks := platformChecks()

	logger.Info("══════════════════════════════════════════════════")
	logger.Info(fmt.Sprintf("  Preflight checks (%s/%s)", runtime.GOOS, runtime.GOARCH))
	logger.Info("──────────────────────────────────────────────────")

	allPassed := true
	for _, c := range checks {
		logger.Info(fmt.Sprintf("  %s %s — %s", c.Icon(), c.Name, c.Detail))
		if c.Status == "fail" && c.Required {
			allPassed = false
		}
	}

	logger.Info("══════════════════════════════════════════════════")
	return allPassed
}
