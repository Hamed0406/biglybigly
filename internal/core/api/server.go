package api

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"github.com/hamed0406/biglybigly/internal/platform"
)

//go:embed all:static
var staticFS embed.FS

type Server struct {
	platform *platform.PlatformImpl
	registry *platform.Registry
}

func NewServer(plat platform.Platform, registry *platform.Registry) http.Handler {
	// Use the platform's mux so module routes are included
	mux := plat.Mux()

	// Platform routes
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
		data, _ := fs.ReadFile(staticFS, "static/index.html")
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	return mux
}
