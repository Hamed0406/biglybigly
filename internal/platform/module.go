// Package platform defines the contract between the Biglybigly core and its
// pluggable tool modules. Every tool implements [Module] and receives a
// [Platform] handle in Init, giving it access to shared infrastructure (DB,
// HTTP mux, auth middleware, SSE broker, logger) without owning any of it.
package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
)

// Module is the contract every tool implements. The lifecycle is:
// Migrate (once, at registration) → Init (register routes) → Start (background
// work, returns when ctx is cancelled). When AgentCapable returns true,
// AgentStart is invoked instead of Start in agent mode.
type Module interface {
	// ID returns a stable, lowercase identifier used as the table prefix
	// (e.g. "dns" → dns_queries) and the API namespace (/api/<id>/...).
	ID() string
	// Name is the human-readable label shown in the UI sidebar.
	Name() string
	// Version is the module's semver string.
	Version() string
	// Icon returns an inline SVG used in the sidebar.
	Icon() string
	// Migrate creates or upgrades the module's tables. Must be idempotent
	// (use CREATE TABLE IF NOT EXISTS) and only touch tables prefixed with ID().
	Migrate(db *sql.DB) error
	// Init registers HTTP routes and other resources. Must not block.
	Init(p Platform) error
	// Start runs background work (collectors, tickers). It must return
	// promptly when ctx is cancelled.
	Start(ctx context.Context) error
	// AgentCapable reports whether the module can run on a remote agent.
	AgentCapable() bool
	// AgentStart is the agent-side entry point, used in place of Start when
	// running in agent mode. It communicates with the server via conn.
	AgentStart(ctx context.Context, conn AgentConn) error
}

// Platform is the dependency-injection handle a module receives in Init.
// Modules should not construct their own DB connections, muxes, or loggers —
// they get them from here so the core can wire everything consistently.
type Platform interface {
	DB() *sql.DB
	Mux() *http.ServeMux
	// Auth returns an HTTP middleware that enforces user authentication.
	Auth() func(http.Handler) http.Handler
	// SSE returns the shared server-sent-events broker for pushing live
	// updates to connected UI clients.
	SSE() SSEBroker
	// Config exposes module-scoped configuration (typically env-var backed).
	Config() ModuleConfig
	Log() *slog.Logger
}

// AgentConn is the bidirectional channel an agent-mode module uses to talk to
// the server over the WebSocket agent protocol.
type AgentConn interface {
	// Send pushes a typed message (e.g. "stats", "dnslogs") to the server.
	Send(msgType string, data any) error
	// Receive returns a channel of inbound messages (typically config updates).
	Receive() <-chan AgentMessage
}

// AgentMessage is a single envelope on the agent ↔ server WebSocket. Data is
// kept as RawMessage so each module can decode its own payload schema.
type AgentMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// SSEBroker fans server-side events out to subscribed UI clients.
type SSEBroker interface {
	// Publish broadcasts an event to every current subscriber.
	Publish(event map[string]any)
	// Subscribe returns a channel of pre-encoded SSE frames for one client.
	Subscribe() <-chan []byte
}

// ModuleConfig provides read-only access to module-specific configuration
// values (typically sourced from environment variables).
type ModuleConfig interface {
	Get(key string) string
}
