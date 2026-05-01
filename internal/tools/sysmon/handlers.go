package sysmon

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// --- Ingest ---

func (m *Module) handleIngest(w http.ResponseWriter, r *http.Request) {
	logger := m.p.Log()

	var payload struct {
		Agent    string         `json:"agent"`
		Snapshot SystemSnapshot `json:"snapshot"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		logger.Warn("sysmon ingest: invalid JSON", "err", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if payload.Agent == "" {
		payload.Agent = "remote"
	}

	db := m.p.DB()
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	snap := payload.Snapshot
	if snap.CollectedAt == 0 {
		snap.CollectedAt = time.Now().Unix()
	}

	result, err := tx.Exec(`
		INSERT INTO sysmon_snapshots (agent_name, cpu_percent, mem_total, mem_used, mem_available,
			load1, load5, load15, os_info, hostname, uptime_secs, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_name, collected_at)
		DO UPDATE SET cpu_percent=excluded.cpu_percent, mem_total=excluded.mem_total,
			mem_used=excluded.mem_used, mem_available=excluded.mem_available,
			load1=excluded.load1, load5=excluded.load5, load15=excluded.load15,
			os_info=excluded.os_info, hostname=excluded.hostname, uptime_secs=excluded.uptime_secs
	`, payload.Agent, snap.CPUPercent, snap.MemTotal, snap.MemUsed, snap.MemAvailable,
		snap.Load1, snap.Load5, snap.Load15, snap.OSInfo, snap.Hostname, snap.UptimeSecs, snap.CollectedAt)
	if err != nil {
		logger.Warn("sysmon ingest: snapshot insert failed", "err", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	snapshotID, _ := result.LastInsertId()

	// Insert disks linked to this snapshot
	for _, d := range snap.Disks {
		tx.Exec(`
			INSERT INTO sysmon_disks (snapshot_id, agent_name, mount_point, fs_type, total_bytes, used_bytes, avail_bytes)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, snapshotID, payload.Agent, d.MountPoint, d.FSType, d.TotalBytes, d.UsedBytes, d.AvailBytes)
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	logger.Debug("sysmon ingest", "agent", payload.Agent, "cpu", snap.CPUPercent, "disks", len(snap.Disks))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- Current (latest snapshot per agent) ---

type SnapshotRow struct {
	ID           int64   `json:"id"`
	AgentName    string  `json:"agent_name"`
	CPUPercent   float64 `json:"cpu_percent"`
	MemTotal     uint64  `json:"mem_total"`
	MemUsed      uint64  `json:"mem_used"`
	MemAvailable uint64  `json:"mem_available"`
	Load1        float64 `json:"load1"`
	Load5        float64 `json:"load5"`
	Load15       float64 `json:"load15"`
	OSInfo       string  `json:"os_info"`
	Hostname     string  `json:"hostname"`
	UptimeSecs   int64   `json:"uptime_secs"`
	CollectedAt  int64   `json:"collected_at"`
}

func (m *Module) handleCurrent(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")

	var rows_result []SnapshotRow

	if agent != "" {
		row := db.QueryRow(`
			SELECT id, agent_name, cpu_percent, mem_total, mem_used, mem_available,
				load1, load5, load15, os_info, hostname, uptime_secs, collected_at
			FROM sysmon_snapshots
			WHERE agent_name = ?
			ORDER BY collected_at DESC LIMIT 1
		`, agent)

		var s SnapshotRow
		err := row.Scan(&s.ID, &s.AgentName, &s.CPUPercent, &s.MemTotal, &s.MemUsed, &s.MemAvailable,
			&s.Load1, &s.Load5, &s.Load15, &s.OSInfo, &s.Hostname, &s.UptimeSecs, &s.CollectedAt)
		if err == nil {
			rows_result = append(rows_result, s)
		}
	} else {
		// Latest per agent using subquery
		rows, err := db.Query(`
			SELECT s.id, s.agent_name, s.cpu_percent, s.mem_total, s.mem_used, s.mem_available,
				s.load1, s.load5, s.load15, s.os_info, s.hostname, s.uptime_secs, s.collected_at
			FROM sysmon_snapshots s
			INNER JOIN (
				SELECT agent_name, MAX(collected_at) AS max_time
				FROM sysmon_snapshots
				GROUP BY agent_name
			) latest ON s.agent_name = latest.agent_name AND s.collected_at = latest.max_time
		`)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var s SnapshotRow
			if err := rows.Scan(&s.ID, &s.AgentName, &s.CPUPercent, &s.MemTotal, &s.MemUsed, &s.MemAvailable,
				&s.Load1, &s.Load5, &s.Load15, &s.OSInfo, &s.Hostname, &s.UptimeSecs, &s.CollectedAt); err != nil {
				continue
			}
			rows_result = append(rows_result, s)
		}
	}

	if rows_result == nil {
		rows_result = []SnapshotRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rows_result)
}

// --- History (time-series for charts) ---

type HistoryPoint struct {
	CPUPercent   float64 `json:"cpu_percent"`
	MemUsed      uint64  `json:"mem_used"`
	MemTotal     uint64  `json:"mem_total"`
	CollectedAt  int64   `json:"collected_at"`
}

func (m *Module) handleHistory(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")
	hoursStr := r.URL.Query().Get("hours")
	hours := 1
	if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 && h <= 168 {
		hours = h
	}

	since := time.Now().Unix() - int64(hours*3600)

	var query string
	var args []interface{}

	if agent != "" {
		query = `SELECT cpu_percent, mem_used, mem_total, collected_at
			FROM sysmon_snapshots WHERE agent_name = ? AND collected_at >= ?
			ORDER BY collected_at ASC`
		args = []interface{}{agent, since}
	} else {
		query = `SELECT cpu_percent, mem_used, mem_total, collected_at
			FROM sysmon_snapshots WHERE collected_at >= ?
			ORDER BY collected_at ASC`
		args = []interface{}{since}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var points []HistoryPoint
	for rows.Next() {
		var p HistoryPoint
		if err := rows.Scan(&p.CPUPercent, &p.MemUsed, &p.MemTotal, &p.CollectedAt); err != nil {
			continue
		}
		points = append(points, p)
	}

	if points == nil {
		points = []HistoryPoint{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}

// --- Disks ---

type DiskRow struct {
	AgentName  string `json:"agent_name"`
	MountPoint string `json:"mount_point"`
	FSType     string `json:"fs_type"`
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
	AvailBytes uint64 `json:"avail_bytes"`
}

func (m *Module) handleDisks(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")

	// Get disks from the latest snapshot(s)
	var query string
	var args []interface{}

	if agent != "" {
		query = `SELECT d.agent_name, d.mount_point, d.fs_type, d.total_bytes, d.used_bytes, d.avail_bytes
			FROM sysmon_disks d
			INNER JOIN (
				SELECT id FROM sysmon_snapshots WHERE agent_name = ?
				ORDER BY collected_at DESC LIMIT 1
			) latest ON d.snapshot_id = latest.id`
		args = []interface{}{agent}
	} else {
		query = `SELECT d.agent_name, d.mount_point, d.fs_type, d.total_bytes, d.used_bytes, d.avail_bytes
			FROM sysmon_disks d
			INNER JOIN (
				SELECT s.id FROM sysmon_snapshots s
				INNER JOIN (
					SELECT agent_name, MAX(collected_at) AS max_time
					FROM sysmon_snapshots GROUP BY agent_name
				) latest ON s.agent_name = latest.agent_name AND s.collected_at = latest.max_time
			) ls ON d.snapshot_id = ls.id`
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var disks []DiskRow
	for rows.Next() {
		var d DiskRow
		if err := rows.Scan(&d.AgentName, &d.MountPoint, &d.FSType, &d.TotalBytes, &d.UsedBytes, &d.AvailBytes); err != nil {
			continue
		}
		disks = append(disks, d)
	}

	if disks == nil {
		disks = []DiskRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(disks)
}

// --- Agents ---

func (m *Module) handleAgents(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()

	rows, err := db.Query(`
		SELECT agent_name, COUNT(*) as snapshot_count, MAX(collected_at) as last_active
		FROM sysmon_snapshots
		GROUP BY agent_name
		ORDER BY last_active DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type AgentInfo struct {
		Name          string `json:"name"`
		SnapshotCount int    `json:"snapshot_count"`
		LastActive    int64  `json:"last_active"`
	}

	var agents []AgentInfo
	for rows.Next() {
		var a AgentInfo
		if err := rows.Scan(&a.Name, &a.SnapshotCount, &a.LastActive); err != nil {
			continue
		}
		agents = append(agents, a)
	}

	if agents == nil {
		agents = []AgentInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

// --- Cleanup ---

func (m *Module) runCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

func (m *Module) cleanup() {
	db := m.p.DB()
	cutoff := time.Now().Unix() - 86400 // 24 hours

	// Delete old disks first (foreign key)
	db.Exec(`DELETE FROM sysmon_disks WHERE snapshot_id IN (
		SELECT id FROM sysmon_snapshots WHERE collected_at < ?
	)`, cutoff)

	result, err := db.Exec(`DELETE FROM sysmon_snapshots WHERE collected_at < ?`, cutoff)
	if err != nil {
		m.p.Log().Warn("sysmon cleanup failed", "err", err)
		return
	}

	if deleted, _ := result.RowsAffected(); deleted > 0 {
		m.p.Log().Debug("sysmon cleanup", "deleted_snapshots", deleted)
	}
}
