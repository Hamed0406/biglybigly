package platform

import (
	"database/sql"
	"log/slog"
	"net/http"
)

type PlatformImpl struct {
	db     *sql.DB
	mux    *http.ServeMux
	logger *slog.Logger
}

func NewPlatform(db *sql.DB, mux *http.ServeMux, logger *slog.Logger) Platform {
	return &PlatformImpl{
		db:     db,
		mux:    mux,
		logger: logger,
	}
}

func (p *PlatformImpl) DB() *sql.DB {
	return p.db
}

func (p *PlatformImpl) Mux() *http.ServeMux {
	return p.mux
}

func (p *PlatformImpl) Auth() func(http.Handler) http.Handler {
	// TODO: implement auth middleware
	return func(h http.Handler) http.Handler {
		return h
	}
}

func (p *PlatformImpl) SSE() SSEBroker {
	// TODO: implement SSE broker
	return nil
}

func (p *PlatformImpl) Config() ModuleConfig {
	// TODO: implement module config
	return nil
}

func (p *PlatformImpl) Log() *slog.Logger {
	return p.logger
}
