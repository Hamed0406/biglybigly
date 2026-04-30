package platform

import (
	"context"
	"database/sql"
	"log/slog"
)

type Registry struct {
	db      *sql.DB
	logger  *slog.Logger
	modules []Module
}

func NewRegistry(db *sql.DB, logger *slog.Logger) *Registry {
	return &Registry{
		db:      db,
		logger:  logger,
		modules: []Module{},
	}
}

func (r *Registry) Register(m Module) error {
	r.logger.Info("Registering module", "id", m.ID(), "name", m.Name())
	
	// Run migrations
	if err := m.Migrate(r.db); err != nil {
		r.logger.Error("Migration failed", "module", m.ID(), "err", err)
		return err
	}
	
	r.modules = append(r.modules, m)
	return nil
}

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

func (r *Registry) Modules() []Module {
	return r.modules
}
