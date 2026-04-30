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

	"github.com/hamed0406/biglybigly/internal/core/storage"
	"github.com/hamed0406/biglybigly/internal/platform"
)

//go:embed all:static
var staticFS embed.FS

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

	return mux
}
