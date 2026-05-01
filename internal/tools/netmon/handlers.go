package netmon

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type FlowRow struct {
	ID         int    `json:"id"`
	AgentName  string `json:"agent_name"`
	Proto      string `json:"proto"`
	LocalIP    string `json:"local_ip"`
	LocalPort  int    `json:"local_port"`
	RemoteIP   string `json:"remote_ip"`
	RemotePort int    `json:"remote_port"`
	Hostname   string `json:"hostname"`
	PID        int    `json:"pid"`
	Process    string `json:"process"`
	State      string `json:"state"`
	Count      int    `json:"count"`
	FirstSeen  int64  `json:"first_seen"`
	LastSeen   int64  `json:"last_seen"`
}

type TopEntry struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type StatsResponse struct {
	TotalFlows   int `json:"total_flows"`
	TotalHosts   int `json:"total_hosts"`
	ActiveNow    int `json:"active_now"`
	UniqueAgents int `json:"unique_agents"`
}

func (m *Module) handleListFlows(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()

	// Query params for filtering
	agent := r.URL.Query().Get("agent")
	search := r.URL.Query().Get("search")
	proto := r.URL.Query().Get("proto")
	limitStr := r.URL.Query().Get("limit")
	limit := 200
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
		limit = l
	}

	query := `
		SELECT id, agent_name, proto, COALESCE(local_ip,''), COALESCE(local_port,0),
		       remote_ip, remote_port, COALESCE(hostname,''), COALESCE(pid,0),
		       COALESCE(process,''), COALESCE(state,''), count, first_seen, last_seen
		FROM netmon_flows
		WHERE 1=1
	`
	var args []interface{}

	if agent != "" {
		query += " AND agent_name = ?"
		args = append(args, agent)
	}
	if proto != "" {
		query += " AND proto = ?"
		args = append(args, proto)
	}
	if search != "" {
		query += " AND (remote_ip LIKE ? OR hostname LIKE ? OR process LIKE ?)"
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern, pattern)
	}

	query += " ORDER BY last_seen DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var flows []FlowRow
	for rows.Next() {
		var f FlowRow
		if err := rows.Scan(&f.ID, &f.AgentName, &f.Proto, &f.LocalIP, &f.LocalPort,
			&f.RemoteIP, &f.RemotePort, &f.Hostname, &f.PID, &f.Process,
			&f.State, &f.Count, &f.FirstSeen, &f.LastSeen); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		flows = append(flows, f)
	}

	if flows == nil {
		flows = []FlowRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(flows)
}

func (m *Module) handleTopHosts(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	query := `
		SELECT COALESCE(NULLIF(hostname,''), remote_ip) AS host, SUM(count) AS total
		FROM netmon_flows
	`
	var args []interface{}
	if agent != "" {
		query += " WHERE agent_name = ?"
		args = append(args, agent)
	}
	query += " GROUP BY host ORDER BY total DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries []TopEntry
	for rows.Next() {
		var e TopEntry
		if err := rows.Scan(&e.Name, &e.Count); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entries = append(entries, e)
	}

	if entries == nil {
		entries = []TopEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (m *Module) handleTopPorts(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")

	query := `SELECT remote_port, SUM(count) AS total FROM netmon_flows`
	var args []interface{}
	if agent != "" {
		query += " WHERE agent_name = ?"
		args = append(args, agent)
	}
	query += " GROUP BY remote_port ORDER BY total DESC LIMIT 20"

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	portNames := map[int]string{
		22: "SSH", 53: "DNS", 80: "HTTP", 443: "HTTPS",
		993: "IMAPS", 995: "POP3S", 587: "SMTP", 8080: "HTTP-Alt",
		3306: "MySQL", 5432: "PostgreSQL", 6379: "Redis", 27017: "MongoDB",
	}

	var entries []TopEntry
	for rows.Next() {
		var port, count int
		if err := rows.Scan(&port, &count); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		name := strconv.Itoa(port)
		if pn, ok := portNames[port]; ok {
			name = pn + " (" + strconv.Itoa(port) + ")"
		}
		entries = append(entries, TopEntry{Name: name, Count: count})
	}

	if entries == nil {
		entries = []TopEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func (m *Module) handleStats(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")
	var stats StatsResponse

	if agent != "" {
		db.QueryRow(`SELECT COUNT(*) FROM netmon_flows WHERE agent_name = ?`, agent).Scan(&stats.TotalFlows)
		db.QueryRow(`SELECT COUNT(DISTINCT COALESCE(NULLIF(hostname,''), remote_ip)) FROM netmon_flows WHERE agent_name = ?`, agent).Scan(&stats.TotalHosts)
		db.QueryRow(`SELECT COUNT(*) FROM netmon_flows WHERE state = 'ESTABLISHED' AND agent_name = ?`, agent).Scan(&stats.ActiveNow)
		stats.UniqueAgents = 1
	} else {
		db.QueryRow(`SELECT COUNT(*) FROM netmon_flows`).Scan(&stats.TotalFlows)
		db.QueryRow(`SELECT COUNT(DISTINCT COALESCE(NULLIF(hostname,''), remote_ip)) FROM netmon_flows`).Scan(&stats.TotalHosts)
		db.QueryRow(`SELECT COUNT(*) FROM netmon_flows WHERE state = 'ESTABLISHED'`).Scan(&stats.ActiveNow)
		db.QueryRow(`SELECT COUNT(DISTINCT agent_name) FROM netmon_flows`).Scan(&stats.UniqueAgents)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleAgents returns a list of known agent names with their flow counts and last seen time
func (m *Module) handleAgents(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()

	rows, err := db.Query(`
		SELECT agent_name, COUNT(*) as flow_count, MAX(last_seen) as last_active
		FROM netmon_flows
		GROUP BY agent_name
		ORDER BY last_active DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type AgentInfo struct {
		Name       string `json:"name"`
		FlowCount  int    `json:"flow_count"`
		LastActive int64  `json:"last_active"`
	}

	var agents []AgentInfo
	for rows.Next() {
		var a AgentInfo
		if err := rows.Scan(&a.Name, &a.FlowCount, &a.LastActive); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		agents = append(agents, a)
	}

	if agents == nil {
		agents = []AgentInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

// handleGraph returns nodes and edges for the network topology visualization
func (m *Module) handleGraph(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")

	query := `
		SELECT agent_name, COALESCE(NULLIF(hostname,''), remote_ip) AS target,
		       remote_port, proto, SUM(count) AS total,
		       MAX(last_seen) AS last_active
		FROM netmon_flows
	`
	var args []interface{}
	if agent != "" {
		query += " WHERE agent_name = ?"
		args = append(args, agent)
	}
	query += " GROUP BY agent_name, target, remote_port, proto ORDER BY total DESC LIMIT 500"

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type GraphNode struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Type  string `json:"type"` // "agent" or "host"
		Size  int    `json:"size"`
	}
	type GraphEdge struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Port   int    `json:"port"`
		Proto  string `json:"proto"`
		Count  int    `json:"count"`
	}

	nodeMap := make(map[string]*GraphNode)
	var edges []GraphEdge

	for rows.Next() {
		var agentName, target, proto string
		var port, count int
		var lastActive int64
		if err := rows.Scan(&agentName, &target, &port, &proto, &count, &lastActive); err != nil {
			continue
		}

		// Ensure agent node exists
		if _, ok := nodeMap[agentName]; !ok {
			nodeMap[agentName] = &GraphNode{
				ID:    agentName,
				Label: agentName,
				Type:  "agent",
				Size:  0,
			}
		}
		nodeMap[agentName].Size += count

		// Ensure host node exists (group by host, not host:port)
		if _, ok := nodeMap[target]; !ok {
			nodeMap[target] = &GraphNode{
				ID:    target,
				Label: target,
				Type:  "host",
				Size:  0,
			}
		}
		nodeMap[target].Size += count

		edges = append(edges, GraphEdge{
			Source: agentName,
			Target: target,
			Port:   port,
			Proto:  proto,
			Count:  count,
		})
	}

	var nodes []GraphNode
	for _, n := range nodeMap {
		nodes = append(nodes, *n)
	}

	if nodes == nil {
		nodes = []GraphNode{}
	}
	if edges == nil {
		edges = []GraphEdge{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	})
}

// handleIngest receives flow data from agents via HTTP (alternative to WebSocket)
func (m *Module) handleIngest(w http.ResponseWriter, r *http.Request) {
	logger := m.p.Log()
	logger.Info("Ingest request received",
		"remote_addr", r.RemoteAddr,
		"content_length", r.ContentLength,
	)

	var payload struct {
		Agent string `json:"agent"`
		Flows []Flow `json:"flows"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		logger.Warn("Ingest: invalid JSON body", "err", err, "remote_addr", r.RemoteAddr)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if payload.Agent == "" {
		payload.Agent = "remote"
	}

	logger.Info("Ingest: processing flows",
		"agent", payload.Agent,
		"flow_count", len(payload.Flows),
		"remote_addr", r.RemoteAddr,
	)

	db := m.p.DB()
	ingested := 0
	errors := 0

	tx, txErr := db.Begin()
	if txErr != nil {
		logger.Warn("Ingest: failed to begin transaction", "err", txErr)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	for _, f := range payload.Flows {
		_, err := tx.Exec(`
			INSERT INTO netmon_flows (agent_name, proto, local_ip, local_port, remote_ip, remote_port, hostname, pid, process, state, count, first_seen, last_seen)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
			ON CONFLICT(agent_name, proto, remote_ip, remote_port)
			DO UPDATE SET count = count + 1, last_seen = ?, state = ?,
				hostname = COALESCE(NULLIF(excluded.hostname,''), hostname),
				process = COALESCE(NULLIF(excluded.process,''), process)
		`, payload.Agent, f.Proto, f.LocalIP, f.LocalPort, f.RemoteIP, f.RemotePort,
			f.Hostname, f.PID, f.Process, f.State, f.SeenAt, f.SeenAt, f.SeenAt, f.State)
		if err != nil {
			logger.Warn("Ingest: DB insert failed",
				"agent", payload.Agent,
				"remote_ip", f.RemoteIP,
				"err", err,
			)
			errors++
		} else {
			ingested++
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Warn("Ingest: commit failed", "err", err)
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	logger.Info("Ingest: complete",
		"agent", payload.Agent,
		"ingested", ingested,
		"errors", errors,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"ingested": ingested, "errors": errors})
}

// --- Hostname History Handlers ---

type HostnameRecord struct {
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
	AgentName string `json:"agent_name"`
	FirstSeen int64  `json:"first_seen"`
	LastSeen  int64  `json:"last_seen"`
	SeenCount int    `json:"seen_count"`
}

// handleHostnames returns all tracked hostname mappings
func (m *Module) handleHostnames(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")
	search := r.URL.Query().Get("search")
	limitStr := r.URL.Query().Get("limit")
	limit := 200
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
		limit = l
	}

	query := `SELECT ip, hostname, agent_name, first_seen, last_seen, seen_count
		FROM netmon_hostname_history WHERE 1=1`
	var args []interface{}

	if agent != "" {
		query += " AND agent_name = ?"
		args = append(args, agent)
	}
	if search != "" {
		query += " AND (ip LIKE ? OR hostname LIKE ?)"
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern)
	}

	query += " ORDER BY last_seen DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []HostnameRecord
	for rows.Next() {
		var r HostnameRecord
		if err := rows.Scan(&r.IP, &r.Hostname, &r.AgentName, &r.FirstSeen, &r.LastSeen, &r.SeenCount); err != nil {
			continue
		}
		records = append(records, r)
	}

	if records == nil {
		records = []HostnameRecord{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

// handleRecentHostnames returns recently discovered hostname mappings
func (m *Module) handleRecentHostnames(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")

	query := `SELECT ip, hostname, agent_name, first_seen, last_seen, seen_count
		FROM netmon_hostname_history WHERE 1=1`
	var args []interface{}

	if agent != "" {
		query += " AND agent_name = ?"
		args = append(args, agent)
	}

	query += " ORDER BY first_seen DESC LIMIT 50"

	rows, err := db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []HostnameRecord
	for rows.Next() {
		var r HostnameRecord
		if err := rows.Scan(&r.IP, &r.Hostname, &r.AgentName, &r.FirstSeen, &r.LastSeen, &r.SeenCount); err != nil {
			continue
		}
		records = append(records, r)
	}

	if records == nil {
		records = []HostnameRecord{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

// handleHostnameLookup returns all hostname records for a specific IP
func (m *Module) handleHostnameLookup(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	ip := r.URL.Query().Get("ip")
	if ip == "" {
		http.Error(w, `{"error":"ip parameter required"}`, http.StatusBadRequest)
		return
	}

	rows, err := db.Query(`
		SELECT ip, hostname, agent_name, first_seen, last_seen, seen_count
		FROM netmon_hostname_history
		WHERE ip = ?
		ORDER BY last_seen DESC
	`, ip)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []HostnameRecord
	for rows.Next() {
		var r HostnameRecord
		if err := rows.Scan(&r.IP, &r.Hostname, &r.AgentName, &r.FirstSeen, &r.LastSeen, &r.SeenCount); err != nil {
			continue
		}
		records = append(records, r)
	}

	if records == nil {
		records = []HostnameRecord{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

// handleHostnameStats returns summary stats for hostname tracking
func (m *Module) handleHostnameStats(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	agent := r.URL.Query().Get("agent")

	type HostnameStats struct {
		TotalMappings int   `json:"total_mappings"`
		UniqueIPs     int   `json:"unique_ips"`
		UniqueNames   int   `json:"unique_names"`
		NewToday      int   `json:"new_today"`
		LastUpdated   int64 `json:"last_updated"`
	}

	var stats HostnameStats
	todayStart := time.Now().Truncate(24 * time.Hour).Unix()

	if agent != "" {
		db.QueryRow(`SELECT COUNT(*) FROM netmon_hostname_history WHERE agent_name = ?`, agent).Scan(&stats.TotalMappings)
		db.QueryRow(`SELECT COUNT(DISTINCT ip) FROM netmon_hostname_history WHERE agent_name = ?`, agent).Scan(&stats.UniqueIPs)
		db.QueryRow(`SELECT COUNT(DISTINCT hostname) FROM netmon_hostname_history WHERE agent_name = ?`, agent).Scan(&stats.UniqueNames)
		db.QueryRow(`SELECT COUNT(*) FROM netmon_hostname_history WHERE agent_name = ? AND first_seen >= ?`, agent, todayStart).Scan(&stats.NewToday)
		db.QueryRow(`SELECT COALESCE(MAX(last_seen), 0) FROM netmon_hostname_history WHERE agent_name = ?`, agent).Scan(&stats.LastUpdated)
	} else {
		db.QueryRow(`SELECT COUNT(*) FROM netmon_hostname_history`).Scan(&stats.TotalMappings)
		db.QueryRow(`SELECT COUNT(DISTINCT ip) FROM netmon_hostname_history`).Scan(&stats.UniqueIPs)
		db.QueryRow(`SELECT COUNT(DISTINCT hostname) FROM netmon_hostname_history`).Scan(&stats.UniqueNames)
		db.QueryRow(`SELECT COUNT(*) FROM netmon_hostname_history WHERE first_seen >= ?`, todayStart).Scan(&stats.NewToday)
		db.QueryRow(`SELECT COALESCE(MAX(last_seen), 0) FROM netmon_hostname_history`).Scan(&stats.LastUpdated)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
