# Biglybigly — Architecture

This document is the authoritative reference for how Biglybigly is structured internally. Read this before writing any code.

---

## Core idea

Biglybigly is a **platform**, not an application. The platform provides:
- Authentication (email/password + OAuth)
- A shared SQLite database
- An HTTP server and React UI shell
- A WebSocket-based agent protocol
- A real-time SSE event bus for the browser

**Tools** are **modules** — Go packages that plug into the platform. A module gets access to the shared database, auth middleware, SSE broker, and HTTP mux. It registers its own routes, runs its own migrations, and can optionally run on a remote agent.

Adding a new tool = write one Go package + one line in `main.go`. The sidebar entry appears automatically.

---

## Modes

```
biglybigly --mode server    (default)
  → starts HTTP server
  → loads and starts all registered modules
  → accepts WebSocket connections from agents
  → serves the React UI

biglybigly --mode agent
  → no HTTP server, no UI
  → connects to the server via WebSocket
  → runs the "agent side" of each AgentCapable module
  → sends stats to server, receives config from server
  → reconnects automatically on disconnect
```

Standalone (single machine, no agents) is just server mode with no agents connected — no special mode needed.

---

## Directory layout

```
biglybigly/
  cmd/
    biglybigly/
      main.go               Entry point. Reads BIGLYBIGLY_MODE, wires modules,
                            starts the platform. Only place module registration happens.

  internal/
    platform/
      module.go             Module and Platform interfaces — the contract every tool signs
      registry.go           Holds registered modules; starts/stops them; exposes /api/modules

    core/
      config/
        config.go           Reads all env vars into a typed Config struct
      storage/
        store.go            Opens SQLite, runs platform migrations, exposes *sql.DB
        migrations.go       Platform-level schema (users, sessions, agent tokens, etc.)
      auth/
        auth.go             Session management, bcrypt, OAuth flows
        middleware.go       requireAuth() — wraps handlers
        oauth.go            Google + GitHub OAuth
      api/
        server.go           Creates *http.ServeMux, mounts core routes, serves SPA
        static/             go:embed target — populated from ui/dist at build time
      sse/
        broker.go           SSE fan-out broker; modules call broker.Publish(event)
      agent/
        server.go           Accepts agent WebSocket connections on /api/agent/connect
        client.go           Agent side: connects to server, handles reconnect backoff
        protocol.go         Message types shared between server and client

    tools/                  Empty for now. Each tool is a subdirectory here.
      example/
        module.go           Reference implementation of the Module interface

  ui/
    src/
      App.tsx               Reads /api/modules → builds sidebar; routes to module pages
      components/
        Shell.tsx           Top bar + collapsible sidebar nav
        AgentsPage.tsx      Lists connected agents and their status
        SettingsPage.tsx    Platform settings (auth, agent tokens, etc.)
      api/
        client.ts           Typed fetch wrappers for platform endpoints
      types.ts              TypeScript types mirroring Go JSON structs

  Dockerfile                3-stage: Node (UI) → Go (binary) → runtime
  docker-compose.yml
  .env.example
  .github/
    workflows/
      ci.yml                Lint, test, build on every push/PR
      release.yml           Multi-platform binaries + Docker on git tag
```

---

## The Module interface

Every tool must implement this interface. Nothing else is required.

```go
// internal/platform/module.go

package platform

import (
    "context"
    "database/sql"
    "net/http"
)

// Module is the contract every tool implements.
type Module interface {
    // ID returns a short, stable, lowercase identifier used in API route prefixes,
    // database table prefixes, and config env var prefixes.
    // Example: "dns", "scanner", "bandwidth"
    // Never change this after the module ships — it's part of the storage schema.
    ID() string

    // Name is the human-readable label shown in the UI sidebar.
    // Example: "DNS Blocker", "Network Scanner"
    Name() string

    // Version is the module's own semantic version, independent of the platform version.
    Version() string

    // Icon returns an SVG string (or icon identifier) used in the sidebar nav.
    Icon() string

    // Migrate is called once at startup, before Init.
    // Create or update the module's database tables here.
    // Use the module's ID as a table prefix to avoid conflicts: dns_queries, dns_blocklists, etc.
    Migrate(db *sql.DB) error

    // Init is called after Migrate. The module receives the Platform handle here
    // and must register all its HTTP routes via p.Mux().
    // Init must not block — start background work in Start().
    Init(p Platform) error

    // Start runs the module's background logic (scan loops, DNS listeners, etc.).
    // It receives a context that is cancelled on shutdown — honour it and return.
    // Start is called in its own goroutine.
    Start(ctx context.Context) error

    // AgentCapable returns true if this module can run on an agent.
    // If false, the agent ignores this module even if the server tells it to run it.
    AgentCapable() bool

    // AgentStart runs the module's agent-side logic.
    // conn lets the module send stats to the server and receive config from it.
    // Only called when running in agent mode and AgentCapable() == true.
    // Runs in its own goroutine; return when ctx is cancelled.
    AgentStart(ctx context.Context, conn AgentConn) error
}

// Platform is what a module receives in Init — its window into the platform.
type Platform interface {
    // DB returns the shared *sql.DB. Use your module ID as a table prefix.
    DB() *sql.DB

    // Mux returns the platform's HTTP mux. Register routes under /api/<your-id>/.
    Mux() *http.ServeMux

    // Auth returns the requireAuth middleware function.
    // Wrap any handler that needs authentication: mux.Handle("/api/dns/...", p.Auth()(handler))
    Auth() func(http.Handler) http.Handler

    // SSE returns the event broker. Call broker.Publish(event) to push to all browser clients.
    SSE() SSEBroker

    // Config returns your module's config block.
    // Env var BIGLYBIGLY_DNS_LISTEN is accessed as p.Config().Get("LISTEN").
    Config() ModuleConfig

    // Log returns a structured logger pre-tagged with your module ID.
    Log() *slog.Logger
}
```

---

## The AgentConn interface

Passed to `AgentStart()` — lets a module talk to the server.

```go
// internal/platform/module.go (continued)

type AgentConn interface {
    // Send pushes a stats payload to the server.
    // data must be JSON-serialisable.
    Send(msgType string, data any) error

    // Receive returns a channel of messages from the server (e.g. config updates).
    // The channel is closed when the connection drops.
    Receive() <-chan AgentMessage
}

type AgentMessage struct {
    Type string          // e.g. "config_update"
    Data json.RawMessage // decode into your module's config struct
}
```

---

## Agent WebSocket protocol

All messages are JSON with a `type` field.

### Handshake

```
Agent → Server:  GET /api/agent/connect
                 Upgrade: websocket
                 Authorization: Bearer <agent-token>

Server → Agent:  { "type": "hello",
                   "server_version": "1.2.0",
                   "modules": ["dns", "scanner"]   ← which modules to activate
                 }
```

### Stats (agent → server, every 30 s per module)

```json
{
  "type": "stats",
  "module": "dns",
  "agent": "office-london",
  "ts": "2026-04-29T10:00:00Z",
  "data": { /* module-defined shape */ }
}
```

### Config update (server → agent, on demand)

```json
{
  "type": "config",
  "module": "dns",
  "data": { /* module-defined shape */ }
}
```

### Keepalive

Standard WebSocket ping/pong every 30 seconds. Server closes the connection if no pong is received within 60 seconds. Agent reconnects with exponential backoff: 1 s → 2 s → 4 s → … → 60 s max.

---

## Platform-level API routes

These are registered by the platform core, not by any module.

| Method | Path | Description |
|---|---|---|
| GET | `/api/modules` | List of loaded modules (id, name, version, icon) |
| GET | `/api/agents` | List of connected agents + status |
| GET | `/api/agents/tokens` | List agent tokens |
| POST | `/api/agents/token` | Create a new agent token |
| DELETE | `/api/agents/token/{id}` | Revoke an agent token |
| GET | `/api/auth/providers` | Available OAuth providers (public) |
| POST | `/api/auth/register` | Email/password registration |
| POST | `/api/auth/login` | Email/password login |
| POST | `/api/auth/logout` | End session |
| GET | `/api/events` | SSE stream for real-time browser push |
| WS | `/api/agent/connect` | Agent WebSocket endpoint |

Each module registers its own routes under `/api/<module-id>/`.

---

## Database conventions

- Platform tables: `users`, `sessions`, `agent_tokens`, `oauth_states`
- Module tables: prefixed with the module ID — `dns_queries`, `dns_blocklists`, `scanner_devices`, etc.
- Each module runs its own migrations in `Migrate(db)` — called in registration order at startup
- Never modify another module's tables — treat them as private

---

## UI conventions

The sidebar is built dynamically by fetching `/api/modules` at startup. Each entry shows the module's `name` and `icon`. Clicking a sidebar entry navigates to `/tools/<module-id>`, which renders `<ModulePage id="dns" />` etc.

Each module's UI lives in `ui/src/tools/<module-id>/`:

```
ui/src/tools/
  dns/
    DnsPage.tsx        — main page component
    components/        — page-specific components
    api.ts             — typed fetch wrappers for /api/dns/... endpoints
```

The shell (`App.tsx`) lazy-loads each tool page so unused modules don't affect initial load time.

---

## Adding a new module — checklist

1. Create `internal/tools/<id>/module.go` implementing `platform.Module`
2. Create DB migrations using `<id>_` table prefix in `Migrate()`
3. Register routes under `/api/<id>/` in `Init()`
4. Create `ui/src/tools/<id>/` with a page component and API client
5. Add one line to `cmd/biglybigly/main.go`:
   ```go
   modules = append(modules, mytool.New())
   ```
6. Done. Sidebar entry and routing appear automatically.

---

## Data flow (server mode)

```
Browser
  └── GET /                 ← React SPA (go:embed)
  └── GET /api/modules      ← sidebar nav items
  └── GET /api/events       ← SSE stream (real-time updates)
  └── GET /api/dns/stats    ← module route (example)

Platform core
  ├── HTTP server
  ├── Auth middleware
  ├── SSE broker
  └── Agent WebSocket server
        └── Connected agents push stats → merged into SQLite
                                        → SSE event to browser

Module (DNS example, server side)
  ├── registered routes: /api/dns/*
  ├── Start(): aggregates stats from local + remote agents
  └── SSE publish on new data

Module (DNS example, agent side)
  ├── AgentStart(): runs local DNS proxy on :53
  ├── every 30s: conn.Send("stats", dnsStats)
  └── conn.Receive(): applies config updates from server
```

---

## Key constraints

- **No CGO** — SQLite via `modernc.org/sqlite` (pure Go). No gcc, no system libs.
- **Go 1.22+** — required by the SQLite driver.
- **Single binary** — the platform and all compiled-in modules ship as one file.
- **go:embed requires static/ to be non-empty** — `static/.gitkeep` satisfies this locally; CI/Docker copies `ui/dist/` in before building.
- **Module IDs are permanent** — they're used as DB table prefixes and API paths. Never rename a shipped module's ID.
- **Agent tokens are pre-shared** — generated on the server, copied to the agent machine. No auto-enrollment (by design: explicit is safer for self-hosted).
- **Modules must not import each other** — use `Platform.SSE()` to publish events and let other modules subscribe, or expose data through HTTP endpoints under `/api/<id>/`.

---

## Security

Known security findings and their recommended fixes are documented in [SECURITY.md](./SECURITY.md).
