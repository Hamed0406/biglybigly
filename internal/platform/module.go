package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
)

// Module is the interface every tool must implement
type Module interface {
	ID() string
	Name() string
	Version() string
	Icon() string
	Migrate(db *sql.DB) error
	Init(p Platform) error
	Start(ctx context.Context) error
	AgentCapable() bool
	AgentStart(ctx context.Context, conn AgentConn) error
}

// Platform is what a module receives in Init
type Platform interface {
	DB() *sql.DB
	Mux() *http.ServeMux
	Auth() func(http.Handler) http.Handler
	SSE() SSEBroker
	Config() ModuleConfig
	Log() *slog.Logger
}

// AgentConn lets agents talk to the server
type AgentConn interface {
	Send(msgType string, data any) error
	Receive() <-chan AgentMessage
}

type AgentMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// SSEBroker handles server-sent events
type SSEBroker interface {
	Publish(event map[string]any)
	Subscribe() <-chan []byte
}

// ModuleConfig reads module-specific env vars
type ModuleConfig interface {
	Get(key string) string
}
