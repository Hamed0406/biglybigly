package urlcheck

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/hamed0406/biglybigly/internal/platform"
)

type Module struct {
	p platform.Platform
}

func New() *Module {
	return &Module{}
}

func (m *Module) ID() string      { return "urlcheck" }
func (m *Module) Name() string    { return "URL Monitor" }
func (m *Module) Version() string { return "0.1.0" }
func (m *Module) Icon() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>`
}

func (m *Module) Migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS urlcheck_urls (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			url         TEXT NOT NULL UNIQUE,
			status      INTEGER,
			last_check  INTEGER,
			created_at  INTEGER NOT NULL,
			updated_at  INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS urlcheck_history (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			url_id      INTEGER NOT NULL,
			status      INTEGER NOT NULL,
			response_time INTEGER,
			error       TEXT,
			checked_at  INTEGER NOT NULL,
			FOREIGN KEY (url_id) REFERENCES urlcheck_urls(id) ON DELETE CASCADE
		);
	`)
	return err
}

func (m *Module) Init(p platform.Platform) error {
	m.p = p
	mux := p.Mux()
	auth := p.Auth()

	// API routes
	mux.Handle("GET /api/urlcheck/urls", auth(http.HandlerFunc(m.handleListURLs)))
	mux.Handle("POST /api/urlcheck/urls", auth(http.HandlerFunc(m.handleAddURL)))
	mux.Handle("DELETE /api/urlcheck/urls/{id}", auth(http.HandlerFunc(m.handleDeleteURL)))
	mux.Handle("GET /api/urlcheck/check/{id}", auth(http.HandlerFunc(m.handleCheckURL)))
	mux.Handle("GET /api/urlcheck/history/{id}", auth(http.HandlerFunc(m.handleGetHistory)))

	return nil
}

func (m *Module) Start(ctx context.Context) error {
	// TODO: periodically check URLs in background
	<-ctx.Done()
	return nil
}

func (m *Module) AgentCapable() bool {
	return false
}

func (m *Module) AgentStart(ctx context.Context, conn platform.AgentConn) error {
	return nil
}
