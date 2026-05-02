package api

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/hamed0406/biglybigly/internal/core/storage"
	"github.com/hamed0406/biglybigly/internal/platform"
)

//go:embed all:static
var staticFS embed.FS

func currentUnix() int64 { return time.Now().Unix() }

type Server struct {
	platform *platform.PlatformImpl
	registry *platform.Registry
}

// GenerateBootstrapToken creates a random token printed to console for first-run security
func GenerateBootstrapToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func NewServer(plat platform.Platform, registry *platform.Registry, bootstrapToken string) http.Handler {
	mux := plat.Mux()
	db := plat.DB()
	logger := plat.Log()

	// Request logging middleware wraps the mux
	loggedMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			logger.Info("API request",
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
			)
		}
		mux.ServeHTTP(w, r)
	})

	// --- Setup API (no auth required, protected by bootstrap token) ---

	mux.HandleFunc("GET /api/setup/status", func(w http.ResponseWriter, r *http.Request) {
		complete := storage.IsSetupComplete(db)
		mode, _ := storage.GetSetting(db, "mode")
		serverURL, _ := storage.GetSetting(db, "server_url")
		instanceName, _ := storage.GetSetting(db, "instance_name")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"setup_complete": complete,
			"mode":           mode,
			"server_url":     serverURL,
			"instance_name":  instanceName,
		})
	})

	mux.HandleFunc("POST /api/setup/complete", func(w http.ResponseWriter, r *http.Request) {
		// Require bootstrap token if setup not yet complete
		if !storage.IsSetupComplete(db) {
			token := r.Header.Get("X-Bootstrap-Token")
			if token == "" || token != bootstrapToken {
				http.Error(w, `{"error":"invalid bootstrap token"}`, http.StatusForbidden)
				return
			}
		}

		var req struct {
			Mode         string `json:"mode"`
			ServerURL    string `json:"server_url"`
			InstanceName string `json:"instance_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
			return
		}

		// Validate
		if req.Mode != "server" && req.Mode != "agent" {
			http.Error(w, `{"error":"mode must be 'server' or 'agent'"}`, http.StatusBadRequest)
			return
		}
		if req.Mode == "agent" && req.ServerURL == "" {
			http.Error(w, `{"error":"server_url is required for agent mode"}`, http.StatusBadRequest)
			return
		}

		// Save in a single transaction
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		settings := map[string]string{
			"mode":           req.Mode,
			"server_url":     req.ServerURL,
			"instance_name":  req.InstanceName,
			"setup_complete": "true",
		}
		for k, v := range settings {
			if _, err := tx.Exec(
				"INSERT INTO platform_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
				k, v,
			); err != nil {
				http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
				return
			}
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}

		slog.Info("Setup completed", "mode", req.Mode, "instance_name", req.InstanceName)

		resp := map[string]interface{}{
			"ok":   true,
			"mode": req.Mode,
		}
		if req.Mode == "agent" {
			resp["message"] = "Configuration saved. Restart biglybigly to connect as agent."
		} else {
			resp["message"] = "Configuration saved. Server mode active."
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// --- Platform API ---

	mux.HandleFunc("GET /api/modules", func(w http.ResponseWriter, r *http.Request) {
		modules := registry.Modules()
		var items []map[string]interface{}
		for _, m := range modules {
			items = append(items, map[string]interface{}{
				"id":      m.ID(),
				"name":    m.Name(),
				"version": m.Version(),
				"icon":    m.Icon(),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})

	// --- Dashboard API ---
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		type AgentSummary struct {
			Name       string  `json:"name"`
			OS         string  `json:"os"`
			CPUPercent float64 `json:"cpu_percent"`
			MemPercent float64 `json:"mem_percent"`
			Uptime     int64   `json:"uptime"`
			LastSeen   int64   `json:"last_seen"`
		}
		type URLStatus struct {
			URL        string `json:"url"`
			StatusCode int    `json:"status_code"`
			LastCheck  int64  `json:"last_check"`
		}
		type Dashboard struct {
			AgentCount     int            `json:"agent_count"`
			AgentsOnline   int            `json:"agents_online"`
			DNSTotal       int            `json:"dns_total"`
			DNSBlocked     int            `json:"dns_blocked"`
			DNSBlockedPct  float64        `json:"dns_blocked_pct"`
			BlocklistSize  int            `json:"blocklist_size"`
			NetFlows       int            `json:"net_flows"`
			TopBlocked     []struct {
				Domain string `json:"domain"`
				Count  int    `json:"count"`
			} `json:"top_blocked"`
			TopQueried []struct {
				Domain string `json:"domain"`
				Count  int    `json:"count"`
			} `json:"top_queried"`
			Agents       []AgentSummary `json:"agents"`
			URLsDown     []URLStatus    `json:"urls_down"`
			RecentBlocks []struct {
				Domain    string `json:"domain"`
				Agent     string `json:"agent"`
				Timestamp int64  `json:"timestamp"`
			} `json:"recent_blocks"`
		}

		var dash Dashboard
		now := fmt.Sprintf("%d", currentUnix())
		_ = now

		// Agent count (from sysmon snapshots, distinct agent names in last 5 min)
		fiveMinAgo := currentUnix() - 300
		oneDayAgo := currentUnix() - 86400

		var agentTotal, agentOnline int
		db.QueryRow(`SELECT COUNT(DISTINCT agent_name) FROM sysmon_snapshots`).Scan(&agentTotal)
		db.QueryRow(`SELECT COUNT(DISTINCT agent_name) FROM sysmon_snapshots WHERE collected_at >= ?`, fiveMinAgo).Scan(&agentOnline)
		dash.AgentCount = agentTotal
		dash.AgentsOnline = agentOnline

		// DNS stats (24h)
		db.QueryRow(`SELECT COUNT(*) FROM dnsfilter_queries WHERE timestamp >= ?`, oneDayAgo).Scan(&dash.DNSTotal)
		db.QueryRow(`SELECT COUNT(*) FROM dnsfilter_queries WHERE timestamp >= ? AND blocked = 1`, oneDayAgo).Scan(&dash.DNSBlocked)
		if dash.DNSTotal > 0 {
			dash.DNSBlockedPct = float64(dash.DNSBlocked) / float64(dash.DNSTotal) * 100
		}

		// Blocklist size
		db.QueryRow(`SELECT COALESCE(SUM(entry_count), 0) FROM dnsfilter_blocklists WHERE enabled = 1`).Scan(&dash.BlocklistSize)

		// Network flows (24h)
		db.QueryRow(`SELECT COUNT(*) FROM netmon_flows WHERE last_seen >= ?`, oneDayAgo).Scan(&dash.NetFlows)

		// Top blocked domains (24h)
		blockedRows, err := db.Query(`
			SELECT domain, COUNT(*) as cnt FROM dnsfilter_queries
			WHERE timestamp >= ? AND blocked = 1
			GROUP BY domain ORDER BY cnt DESC LIMIT 5
		`, oneDayAgo)
		if err == nil {
			for blockedRows.Next() {
				var d struct {
					Domain string `json:"domain"`
					Count  int    `json:"count"`
				}
				blockedRows.Scan(&d.Domain, &d.Count)
				dash.TopBlocked = append(dash.TopBlocked, d)
			}
			blockedRows.Close()
		}

		// Top queried domains (24h)
		queriedRows, err := db.Query(`
			SELECT domain, COUNT(*) as cnt FROM dnsfilter_queries
			WHERE timestamp >= ?
			GROUP BY domain ORDER BY cnt DESC LIMIT 5
		`, oneDayAgo)
		if err == nil {
			for queriedRows.Next() {
				var d struct {
					Domain string `json:"domain"`
					Count  int    `json:"count"`
				}
				queriedRows.Scan(&d.Domain, &d.Count)
				dash.TopQueried = append(dash.TopQueried, d)
			}
			queriedRows.Close()
		}

		// Agent summaries (latest snapshot per agent)
		agentRows, err := db.Query(`
			SELECT s.agent_name, s.os, s.cpu_percent, s.mem_used, s.mem_total, s.uptime_secs, s.collected_at
			FROM sysmon_snapshots s
			INNER JOIN (
				SELECT agent_name, MAX(collected_at) as max_time
				FROM sysmon_snapshots GROUP BY agent_name
			) latest ON s.agent_name = latest.agent_name AND s.collected_at = latest.max_time
			ORDER BY s.collected_at DESC
		`)
		if err == nil {
			for agentRows.Next() {
				var a AgentSummary
				var memUsed, memTotal int64
				agentRows.Scan(&a.Name, &a.OS, &a.CPUPercent, &memUsed, &memTotal, &a.Uptime, &a.LastSeen)
				if memTotal > 0 {
					a.MemPercent = float64(memUsed) / float64(memTotal) * 100
				}
				dash.Agents = append(dash.Agents, a)
			}
			agentRows.Close()
		}

		// URLs down (last check not 200)
		urlRows, err := db.Query(`SELECT url, last_status, last_check FROM urlcheck_urls WHERE last_status != 200 AND last_status != 0`)
		if err == nil {
			for urlRows.Next() {
				var u URLStatus
				urlRows.Scan(&u.URL, &u.StatusCode, &u.LastCheck)
				dash.URLsDown = append(dash.URLsDown, u)
			}
			urlRows.Close()
		}

		// Recent blocks (last 10)
		recentRows, err := db.Query(`
			SELECT domain, agent_name, timestamp FROM dnsfilter_queries
			WHERE blocked = 1 ORDER BY timestamp DESC LIMIT 10
		`)
		if err == nil {
			for recentRows.Next() {
				var b struct {
					Domain    string `json:"domain"`
					Agent     string `json:"agent"`
					Timestamp int64  `json:"timestamp"`
				}
				recentRows.Scan(&b.Domain, &b.Agent, &b.Timestamp)
				dash.RecentBlocks = append(dash.RecentBlocks, b)
			}
			recentRows.Close()
		}

		// Ensure non-nil slices
		if dash.TopBlocked == nil {
			dash.TopBlocked = make([]struct {
				Domain string `json:"domain"`
				Count  int    `json:"count"`
			}, 0)
		}
		if dash.TopQueried == nil {
			dash.TopQueried = make([]struct {
				Domain string `json:"domain"`
				Count  int    `json:"count"`
			}, 0)
		}
		if dash.Agents == nil {
			dash.Agents = []AgentSummary{}
		}
		if dash.URLsDown == nil {
			dash.URLsDown = []URLStatus{}
		}
		if dash.RecentBlocks == nil {
			dash.RecentBlocks = make([]struct {
				Domain    string `json:"domain"`
				Agent     string `json:"agent"`
				Timestamp int64  `json:"timestamp"`
			}, 0)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dash)
	})

	// Serve static assets (embedded UI)
	distFS, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /assets/", http.FileServer(http.FS(distFS)))

	// SPA fallback: serve index.html for all non-API, non-asset paths
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(staticFS, "static/index.html")
		if err != nil {
			// No UI built — show a helpful message
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<html><body><h1>Biglybigly</h1><p>UI not built. Use <code>cd ui && npm run build</code> or access <code>/api/setup/status</code></p></body></html>")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	return loggedMux
}
