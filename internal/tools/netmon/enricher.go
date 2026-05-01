package netmon

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"strings"
	"time"
)

// runHostnameEnricher periodically scans flows and builds hostname history.
// It records IP↔hostname mappings per agent, tracking when mappings appear and change.
func (m *Module) runHostnameEnricher(ctx context.Context) {
	db := m.p.DB()
	logger := m.p.Log()

	// Run immediately on start, then every 60 seconds
	m.enrichHostnames(ctx, db, logger)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.enrichHostnames(ctx, db, logger)
		}
	}
}

func (m *Module) enrichHostnames(ctx context.Context, db *sql.DB, logger *slog.Logger) {
	// Get all flows that have a hostname already set (from agent-side resolution)
	rows, err := db.Query(`
		SELECT DISTINCT agent_name, remote_ip, hostname
		FROM netmon_flows
		WHERE hostname != '' AND hostname IS NOT NULL
	`)
	if err != nil {
		logger.Warn("Hostname enricher: query failed", "err", err)
		return
	}
	defer rows.Close()

	now := time.Now().Unix()
	inserted := 0
	updated := 0

	for rows.Next() {
		var agentName, ip, hostname string
		if err := rows.Scan(&agentName, &ip, &hostname); err != nil {
			continue
		}

		res, err := db.ExecContext(ctx, `
			INSERT INTO netmon_hostname_history (ip, hostname, agent_name, first_seen, last_seen, seen_count)
			VALUES (?, ?, ?, ?, ?, 1)
			ON CONFLICT(ip, hostname, agent_name)
			DO UPDATE SET last_seen = ?, seen_count = seen_count + 1
		`, ip, hostname, agentName, now, now, now)
		if err != nil {
			continue
		}
		if affected, _ := res.RowsAffected(); affected > 0 {
			updated++
		}
	}

	// Also do server-side reverse DNS for IPs without hostnames (batch, rate-limited)
	unresolved, err := db.Query(`
		SELECT DISTINCT remote_ip FROM netmon_flows
		WHERE (hostname = '' OR hostname IS NULL)
		AND remote_ip NOT IN (SELECT ip FROM netmon_dns_cache WHERE resolved_at > ?)
		LIMIT 50
	`, now-3600) // re-resolve every hour
	if err != nil {
		return
	}
	defer unresolved.Close()

	resolved := 0
	for unresolved.Next() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var ip string
		if err := unresolved.Scan(&ip); err != nil {
			continue
		}

		names, err := net.LookupAddr(ip)
		hostname := ""
		if err == nil && len(names) > 0 {
			hostname = strings.TrimSuffix(names[0], ".")
		}

		// Update dns cache
		db.ExecContext(ctx, `
			INSERT INTO netmon_dns_cache (ip, hostname, resolved_at)
			VALUES (?, ?, ?)
			ON CONFLICT(ip) DO UPDATE SET hostname = ?, resolved_at = ?
		`, ip, hostname, now, hostname, now)

		// Update flows with the resolved hostname
		if hostname != "" {
			db.ExecContext(ctx, `
				UPDATE netmon_flows SET hostname = ?
				WHERE remote_ip = ? AND (hostname = '' OR hostname IS NULL)
			`, hostname, ip)

			// Track in history (attribute to all agents that have this IP)
			agentRows, err := db.Query(`
				SELECT DISTINCT agent_name FROM netmon_flows WHERE remote_ip = ?
			`, ip)
			if err == nil {
				for agentRows.Next() {
					var agent string
					agentRows.Scan(&agent)
					db.ExecContext(ctx, `
						INSERT INTO netmon_hostname_history (ip, hostname, agent_name, first_seen, last_seen, seen_count)
						VALUES (?, ?, ?, ?, ?, 1)
						ON CONFLICT(ip, hostname, agent_name)
						DO UPDATE SET last_seen = ?, seen_count = seen_count + 1
					`, ip, hostname, agent, now, now, now)
				}
				agentRows.Close()
			}

			resolved++
		}

		inserted++
	}

	if updated > 0 || resolved > 0 {
		logger.Debug("Hostname enricher", "agent_mappings_updated", updated, "server_resolved", resolved)
	}
}
