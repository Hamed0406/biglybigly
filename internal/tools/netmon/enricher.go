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

// enrichHostnames runs one pass: it folds every (agent, ip, hostname) seen
// in netmon_flows into netmon_hostname_history (so changes over time are
// preserved per agent), then resolves up to 50 still-unresolved IPs via
// reverse DNS, caches results in netmon_dns_cache, back-fills the hostname
// column on netmon_flows, and records the new mappings in the history table
// for every agent that has talked to that IP.
func (m *Module) enrichHostnames(ctx context.Context, db *sql.DB, logger *slog.Logger) {
	// Collect flows with hostnames into memory first to avoid holding the DB connection
	type flowMapping struct {
		agentName string
		ip        string
		hostname  string
	}

	rows, err := db.Query(`
		SELECT DISTINCT agent_name, remote_ip, hostname
		FROM netmon_flows
		WHERE hostname != '' AND hostname IS NOT NULL
	`)
	if err != nil {
		logger.Warn("Hostname enricher: query failed", "err", err)
		return
	}

	var mappings []flowMapping
	for rows.Next() {
		var fm flowMapping
		if err := rows.Scan(&fm.agentName, &fm.ip, &fm.hostname); err != nil {
			continue
		}
		mappings = append(mappings, fm)
	}
	rows.Close()

	now := time.Now().Unix()
	updated := 0

	for _, fm := range mappings {
		_, err := db.ExecContext(ctx, `
			INSERT INTO netmon_hostname_history (ip, hostname, agent_name, first_seen, last_seen, seen_count)
			VALUES (?, ?, ?, ?, ?, 1)
			ON CONFLICT(ip, hostname, agent_name)
			DO UPDATE SET last_seen = ?, seen_count = seen_count + 1
		`, fm.ip, fm.hostname, fm.agentName, now, now, now)
		if err != nil {
			continue
		}
		updated++
	}

	// Collect unresolved IPs into memory first
	unresolved, err := db.Query(`
		SELECT DISTINCT remote_ip FROM netmon_flows
		WHERE (hostname = '' OR hostname IS NULL)
		AND remote_ip NOT IN (SELECT ip FROM netmon_dns_cache WHERE resolved_at > ?)
		LIMIT 50
	`, now-3600)
	if err != nil {
		return
	}

	var unresolvedIPs []string
	for unresolved.Next() {
		var ip string
		if err := unresolved.Scan(&ip); err != nil {
			continue
		}
		unresolvedIPs = append(unresolvedIPs, ip)
	}
	unresolved.Close()

	resolved := 0
	for _, ip := range unresolvedIPs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		names, err := net.LookupAddr(ip)
		hostname := ""
		if err == nil && len(names) > 0 {
			hostname = strings.TrimSuffix(names[0], ".")
		}

		db.ExecContext(ctx, `
			INSERT INTO netmon_dns_cache (ip, hostname, resolved_at)
			VALUES (?, ?, ?)
			ON CONFLICT(ip) DO UPDATE SET hostname = ?, resolved_at = ?
		`, ip, hostname, now, hostname, now)

		if hostname != "" {
			db.ExecContext(ctx, `
				UPDATE netmon_flows SET hostname = ?
				WHERE remote_ip = ? AND (hostname = '' OR hostname IS NULL)
			`, hostname, ip)

			// Get agents for this IP
			agentRows, err := db.Query(`
				SELECT DISTINCT agent_name FROM netmon_flows WHERE remote_ip = ?
			`, ip)
			if err == nil {
				var agents []string
				for agentRows.Next() {
					var agent string
					agentRows.Scan(&agent)
					agents = append(agents, agent)
				}
				agentRows.Close()

				for _, agent := range agents {
					db.ExecContext(ctx, `
						INSERT INTO netmon_hostname_history (ip, hostname, agent_name, first_seen, last_seen, seen_count)
						VALUES (?, ?, ?, ?, ?, 1)
						ON CONFLICT(ip, hostname, agent_name)
						DO UPDATE SET last_seen = ?, seen_count = seen_count + 1
					`, ip, hostname, agent, now, now, now)
				}
			}

			resolved++
		}
	}

	if updated > 0 || resolved > 0 {
		logger.Debug("Hostname enricher", "agent_mappings_updated", updated, "server_resolved", resolved)
	}
}
