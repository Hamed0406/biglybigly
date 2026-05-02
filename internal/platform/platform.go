package platform

import (
	"database/sql"
	"log/slog"
	"net/http"
)

// PlatformImpl is the concrete [Platform] passed to modules. It bundles the
// shared DB, HTTP mux, and logger so modules don't need their own DI plumbing.
// Auth, SSE, and Config are stubbed for now and will be wired in as the core
// matures.
type PlatformImpl struct {
	db     *sql.DB
	mux    *http.ServeMux
	logger *slog.Logger
}

// NewPlatform constructs a [Platform] backed by the given shared resources.
// The same instance is handed to every module's Init.
func NewPlatform(db *sql.DB, mux *http.ServeMux, logger *slog.Logger) Platform {
	return &PlatformImpl{
		db:     db,
		mux:    mux,
		logger: logger,
	}
}

// DB returns the shared SQLite handle. All modules use the same connection
// (a single writer) to avoid "database is locked" under modernc.org/sqlite.
func (p *PlatformImpl) DB() *sql.DB {
	return p.db
}

// Mux returns the shared HTTP mux. Modules register their /api/<id>/... routes
// directly on it during Init.
func (p *PlatformImpl) Mux() *http.ServeMux {
	return p.mux
}

// Auth returns an HTTP middleware enforcing authentication. Currently a
// pass-through stub — see SECURITY.md (C1).
func (p *PlatformImpl) Auth() func(http.Handler) http.Handler {
	// TODO: implement auth middleware
	return func(h http.Handler) http.Handler {
		return h
	}
}

// SSE returns the SSE broker. Not yet wired up.
func (p *PlatformImpl) SSE() SSEBroker {
	// TODO: implement SSE broker
	return nil
}

// Config returns the module-scoped config reader. Not yet wired up.
func (p *PlatformImpl) Config() ModuleConfig {
	// TODO: implement module config
	return nil
}

// Log returns the shared structured logger.
func (p *PlatformImpl) Log() *slog.Logger {
	return p.logger
}
