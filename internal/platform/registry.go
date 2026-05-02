package platform

import (
	"context"
	"database/sql"
	"log/slog"
)

// Registry tracks all modules registered with the platform and drives their
// migration → init → start lifecycle. Registration order is preserved so that
// migrations and inits run deterministically; any failure short-circuits.
type Registry struct {
	db      *sql.DB
	logger  *slog.Logger
	modules []Module
}

// NewRegistry creates an empty registry bound to the given DB and logger.
func NewRegistry(db *sql.DB, logger *slog.Logger) *Registry {
	return &Registry{
		db:      db,
		logger:  logger,
		modules: []Module{},
	}
}

// Register adds a module to the registry and immediately runs its migrations.
// Migrating at registration time (rather than at Start) ensures the schema is
// ready before the HTTP server begins serving requests.
func (r *Registry) Register(m Module) error {
	r.logger.Info("Registering module", "id", m.ID(), "name", m.Name())

	if err := m.Migrate(r.db); err != nil {
		r.logger.Error("Migration failed", "module", m.ID(), "err", err)
		return err
	}

	r.modules = append(r.modules, m)
	return nil
}

// Start invokes Init synchronously on every module (so route registration is
// complete before the server accepts traffic), then launches each module's
// Start in its own goroutine. context.Canceled is treated as a clean shutdown.
func (r *Registry) Start(ctx context.Context, p Platform) error {
	for _, m := range r.modules {
		if err := m.Init(p); err != nil {
			r.logger.Error("Init failed", "module", m.ID(), "err", err)
			return err
		}

		go func(mod Module) {
			if err := mod.Start(ctx); err != nil && err != context.Canceled {
				r.logger.Error("Start error", "module", mod.ID(), "err", err)
			}
		}(m)
	}
	return nil
}

// Modules returns the registered modules in registration order. Used by the
// /api/modules endpoint to build the UI sidebar.
func (r *Registry) Modules() []Module {
	return r.modules
}
