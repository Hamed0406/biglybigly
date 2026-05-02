package dnsfilter

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// --- Stats ---

type StatsResponse struct {
	TotalQueries   int     `json:"total_queries"`
	BlockedQueries int     `json:"blocked_queries"`
	BlockedPercent float64 `json:"blocked_percent"`
	UniqueClients  int     `json:"unique_clients"`
	UniqueDomains  int     `json:"unique_domains"`
	BlocklistSize  int     `json:"blocklist_size"`
	TopBlocked     []TopDomain `json:"top_blocked"`
	TopQueried     []TopDomain `json:"top_queried"`
}

type TopDomain struct {
	Domain string `json:"domain"`
	Count  int    `json:"count"`
}

func (m *Module) handleStats(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")
	hoursStr := r.URL.Query().Get("hours")
	hours := 24
	if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 && h <= 168 {
		hours = h
	}

	since := time.Now().Unix() - int64(hours*3600)
	var stats StatsResponse

	agentFilter := ""
	var args []interface{}
	args = append(args, since)
	if agent != "" {
		agentFilter = " AND agent_name = ?"
		args = append(args, agent)
	}

	db.QueryRow(`SELECT COUNT(*) FROM dnsfilter_queries WHERE timestamp >= ?`+agentFilter, args...).Scan(&stats.TotalQueries)
	db.QueryRow(`SELECT COUNT(*) FROM dnsfilter_queries WHERE timestamp >= ? AND blocked = 1`+agentFilter, args...).Scan(&stats.BlockedQueries)
	db.QueryRow(`SELECT COUNT(DISTINCT client_ip) FROM dnsfilter_queries WHERE timestamp >= ?`+agentFilter, args...).Scan(&stats.UniqueClients)
	db.QueryRow(`SELECT COUNT(DISTINCT domain) FROM dnsfilter_queries WHERE timestamp >= ?`+agentFilter, args...).Scan(&stats.UniqueDomains)

	if stats.TotalQueries > 0 {
		stats.BlockedPercent = float64(stats.BlockedQueries) / float64(stats.TotalQueries) * 100
	}

	// Blocklist size
	if m.blocklist != nil {
		stats.BlocklistSize = m.blocklist.TotalBlocked()
	} else {
		db.QueryRow(`SELECT COALESCE(SUM(entry_count), 0) FROM dnsfilter_blocklists WHERE enabled = 1`).Scan(&stats.BlocklistSize)
	}

	// Top blocked
	topArgs := []interface{}{since}
	topFilter := ""
	if agent != "" {
		topFilter = " AND agent_name = ?"
		topArgs = append(topArgs, agent)
	}

	blockedRows, err := db.Query(`
		SELECT domain, COUNT(*) as cnt FROM dnsfilter_queries
		WHERE timestamp >= ? AND blocked = 1`+topFilter+`
		GROUP BY domain ORDER BY cnt DESC LIMIT 10
	`, topArgs...)
	if err == nil {
		for blockedRows.Next() {
			var td TopDomain
			blockedRows.Scan(&td.Domain, &td.Count)
			stats.TopBlocked = append(stats.TopBlocked, td)
		}
		blockedRows.Close()
	}

	// Top queried
	queriedRows, err := db.Query(`
		SELECT domain, COUNT(*) as cnt FROM dnsfilter_queries
		WHERE timestamp >= ?`+topFilter+`
		GROUP BY domain ORDER BY cnt DESC LIMIT 10
	`, topArgs...)
	if err == nil {
		for queriedRows.Next() {
			var td TopDomain
			queriedRows.Scan(&td.Domain, &td.Count)
			stats.TopQueried = append(stats.TopQueried, td)
		}
		queriedRows.Close()
	}

	if stats.TopBlocked == nil {
		stats.TopBlocked = []TopDomain{}
	}
	if stats.TopQueried == nil {
		stats.TopQueried = []TopDomain{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// --- Query Log ---

type QueryRow struct {
	ID         int    `json:"id"`
	AgentName  string `json:"agent_name"`
	Domain     string `json:"domain"`
	QType      string `json:"qtype"`
	ClientIP   string `json:"client_ip"`
	Blocked    bool   `json:"blocked"`
	UpstreamMs int64  `json:"upstream_ms"`
	Answer     string `json:"answer"`
	Timestamp  int64  `json:"timestamp"`
}

func (m *Module) handleQueries(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")
	search := r.URL.Query().Get("search")
	blockedOnly := r.URL.Query().Get("blocked") == "true"
	limitStr := r.URL.Query().Get("limit")
	limit := 200
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
		limit = l
	}

	query := `SELECT id, agent_name, domain, qtype, client_ip, blocked, upstream_ms, answer, timestamp
		FROM dnsfilter_queries WHERE 1=1`
	var args []interface{}

	if agent != "" {
		query += " AND agent_name = ?"
		args = append(args, agent)
	}
	if search != "" {
		query += " AND (domain LIKE ? OR answer LIKE ?)"
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern)
	}
	if blockedOnly {
		query += " AND blocked = 1"
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var queries []QueryRow
	for rows.Next() {
		var q QueryRow
		var blocked int
		if err := rows.Scan(&q.ID, &q.AgentName, &q.Domain, &q.QType, &q.ClientIP,
			&blocked, &q.UpstreamMs, &q.Answer, &q.Timestamp); err != nil {
			continue
		}
		q.Blocked = blocked == 1
		queries = append(queries, q)
	}

	if queries == nil {
		queries = []QueryRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queries)
}

// --- Agents ---

func (m *Module) handleAgents(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()

	rows, err := db.Query(`
		SELECT agent_name, COUNT(*) as query_count,
			SUM(CASE WHEN blocked = 1 THEN 1 ELSE 0 END) as blocked_count,
			MAX(timestamp) as last_active
		FROM dnsfilter_queries
		GROUP BY agent_name
		ORDER BY last_active DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type AgentInfo struct {
		Name         string `json:"name"`
		QueryCount   int    `json:"query_count"`
		BlockedCount int    `json:"blocked_count"`
		LastActive   int64  `json:"last_active"`
	}

	var agents []AgentInfo
	for rows.Next() {
		var a AgentInfo
		if err := rows.Scan(&a.Name, &a.QueryCount, &a.BlockedCount, &a.LastActive); err != nil {
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

// --- Blocklist Management ---

type BlocklistRow struct {
	ID          int    `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	EntryCount  int    `json:"entry_count"`
	LastUpdated int64  `json:"last_updated"`
	CreatedAt   int64  `json:"created_at"`
}

func (m *Module) handleListBlocklists(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()

	rows, err := db.Query(`SELECT id, url, name, enabled, entry_count, last_updated, created_at FROM dnsfilter_blocklists ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var lists []BlocklistRow
	for rows.Next() {
		var b BlocklistRow
		var enabled int
		if err := rows.Scan(&b.ID, &b.URL, &b.Name, &enabled, &b.EntryCount, &b.LastUpdated, &b.CreatedAt); err != nil {
			continue
		}
		b.Enabled = enabled == 1
		lists = append(lists, b)
	}

	if lists == nil {
		lists = []BlocklistRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(lists)
}

func (m *Module) handleAddBlocklist(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()

	var req struct {
		URL  string `json:"url"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, `{"error":"url is required"}`, http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		req.Name = req.URL
	}

	now := time.Now().Unix()
	_, err := db.Exec(`INSERT INTO dnsfilter_blocklists (url, name, enabled, created_at) VALUES (?, ?, 1, ?)`,
		req.URL, req.Name, now)
	if err != nil {
		http.Error(w, `{"error":"blocklist already exists or database error"}`, http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *Module) handleDeleteBlocklist(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	id := r.PathValue("id")

	_, err := db.Exec(`DELETE FROM dnsfilter_blocklists WHERE id = ?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *Module) handleRefreshBlocklists(w http.ResponseWriter, r *http.Request) {
	if m.blocklist == nil {
		http.Error(w, `{"error":"blocklist manager not initialized"}`, http.StatusServiceUnavailable)
		return
	}

	go func() {
		if err := m.blocklist.LoadFromDB(m.p.DB()); err != nil {
			m.p.Log().Warn("Blocklist refresh failed", "err", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "refreshing"})
}

// --- Custom Rules ---

type RuleRow struct {
	ID        int    `json:"id"`
	Domain    string `json:"domain"`
	Action    string `json:"action"`
	CreatedAt int64  `json:"created_at"`
}

func (m *Module) handleListRules(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()

	rows, err := db.Query(`SELECT id, domain, action, created_at FROM dnsfilter_custom_rules ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var rules []RuleRow
	for rows.Next() {
		var rule RuleRow
		if err := rows.Scan(&rule.ID, &rule.Domain, &rule.Action, &rule.CreatedAt); err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	if rules == nil {
		rules = []RuleRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rules)
}

func (m *Module) handleAddRule(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()

	var req struct {
		Domain string `json:"domain"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Domain == "" {
		http.Error(w, `{"error":"domain is required"}`, http.StatusBadRequest)
		return
	}
	if req.Action != "block" && req.Action != "allow" {
		req.Action = "block"
	}

	// Extract domain from URL if user pasted a full URL
	domain := extractDomainFromInput(req.Domain)
	if domain == "" {
		http.Error(w, `{"error":"could not extract a valid domain"}`, http.StatusBadRequest)
		return
	}

	now := time.Now().Unix()
	_, err := db.Exec(`INSERT OR IGNORE INTO dnsfilter_custom_rules (domain, action, created_at) VALUES (?, ?, ?)`,
		domain, req.Action, now)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	// Reload blocklist if available
	if m.blocklist != nil {
		go m.blocklist.LoadFromDB(m.p.DB())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *Module) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	id := r.PathValue("id")

	_, err := db.Exec(`DELETE FROM dnsfilter_custom_rules WHERE id = ?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload blocklist
	if m.blocklist != nil {
		go m.blocklist.LoadFromDB(m.p.DB())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- Ingest (agent submits query logs) ---

func (m *Module) handleIngest(w http.ResponseWriter, r *http.Request) {
	logger := m.p.Log()

	var payload struct {
		Agent   string     `json:"agent"`
		Queries []QueryLog `json:"queries"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		logger.Warn("dnsfilter ingest: invalid JSON", "err", err)
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

	ingested := 0
	for _, q := range payload.Queries {
		blocked := 0
		if q.Blocked {
			blocked = 1
		}
		_, err := tx.Exec(`
			INSERT INTO dnsfilter_queries (agent_name, domain, qtype, client_ip, blocked, upstream_ms, answer, timestamp)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, payload.Agent, q.Domain, q.QType, q.ClientIP, blocked, q.UpstreamMs, q.Answer, q.Timestamp)
		if err != nil {
			continue
		}
		ingested++
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	logger.Debug("dnsfilter ingest", "agent", payload.Agent, "queries", ingested)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"ingested": ingested})
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
			db := m.p.DB()
			cutoff := time.Now().Unix() - 7*86400 // 7 days
			result, err := db.Exec(`DELETE FROM dnsfilter_queries WHERE timestamp < ?`, cutoff)
			if err != nil {
				m.p.Log().Warn("dnsfilter cleanup failed", "err", err)
				continue
			}
			if deleted, _ := result.RowsAffected(); deleted > 0 {
				m.p.Log().Debug("dnsfilter cleanup", "deleted_queries", deleted)
			}
		}
	}
}
