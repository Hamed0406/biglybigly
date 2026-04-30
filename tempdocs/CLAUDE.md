# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

Biglybigly is a self-hosted network management platform built around pluggable modules (tools). The platform provides auth, a shared SQLite database, an HTTP server, a React UI shell, and a WebSocket-based agent protocol. Tools are Go packages that implement the `Module` interface and slot into the platform.

**Two modes:**
- `server` — full UI, database, accepts remote agents (default)
- `agent` — no UI, connects to a server, runs module logic remotely

## Directory layout

```
cmd/
  biglybigly/
    main.go               Entry point. Reads BIGLYBIGLY_MODE, registers modules, starts platform.
                          This is the ONLY place module registration happens.

internal/
  platform/
    module.go             Module and Platform interfaces — the contract every tool signs.
    registry.go           Registers, migrates, inits, and starts modules.
    platform.go           Platform implementation passed to each module's Init().

  core/
    config/config.go      Typed config from env vars / .env file.
    storage/store.go      Opens SQLite, runs platform migrations, exposes *sql.DB.
    auth/                 Session management, bcrypt, Google/GitHub OAuth.
    api/server.go         HTTP server, SPA serving, mounts /api/modules and /api/agents routes.
    api/static/           go:embed target — populated from ui/dist at build time.
    sse/broker.go         SSE fan-out broker. Modules call broker.Publish(event).
    agent/server.go       Accepts agent WebSocket connections on /api/agent/connect.
    agent/client.go       Agent side — connects to server, reconnects with backoff.
    agent/protocol.go     JSON message types shared between server and client.

  tools/                  One subdirectory per module. Currently empty.
    example/module.go     Reference Module implementation.

ui/
  src/
    App.tsx               Fetches /api/modules → builds sidebar, routes /tools/<id> to pages.
    components/
      Shell.tsx           Top bar + sidebar nav.
      AgentsPage.tsx      Shows connected agents and their status.
      SettingsPage.tsx    Platform settings (auth tokens, agent tokens, etc.).
    tools/                One subdirectory per module's UI.
      <module-id>/
        <ModuleName>Page.tsx
        api.ts            Typed fetch wrappers for /api/<module-id>/*
    api/client.ts         Platform-level fetch wrappers.
    types.ts              TypeScript mirrors of Go JSON types.
```

## Commands

### Go — run from repo root
```bash
go build ./...
go test ./...
go vet ./...

# Run with UI already built:
cp -r ui/dist/* internal/core/api/static/
go run ./cmd/biglybigly

# Agent mode:
BIGLYBIGLY_MODE=agent \
BIGLYBIGLY_SERVER_URL=https://example.com \
BIGLYBIGLY_AGENT_TOKEN=abc123 \
go run ./cmd/biglybigly
```

### UI — run from `ui/`
```bash
npm install
npm run dev      # Vite dev server on :5173, proxies /api → :8080
npm run build    # output: dist/ — copy to internal/core/api/static/ for embedded binary
```

### Docker — run from repo root
```bash
docker compose build
docker compose up -d
```

### Release — cut a new version
```bash
git tag v1.0.0 && git push origin v1.0.0
# GitHub Actions builds Docker (linux/amd64 + arm64) + native binaries for all platforms.
```

## Key constraints

- **No CGO** — SQLite via `modernc.org/sqlite` (pure Go). No gcc, no system libs.
- **Go 1.22+** — required by the SQLite driver.
- **Module IDs are permanent** — used as DB table prefixes and URL paths. Never rename a shipped module's ID.
- **Module interface stability** — adding methods to the Module interface breaks all existing modules. Discuss before changing it.
- **Modules must not import each other** — use Platform.SSE() events or expose data through `/api/<id>/` endpoints for cross-module data sharing.
- **All module DB tables must use `<id>_` prefix** — prevents conflicts between modules.
- **All module API routes must live under `/api/<id>/`** — prevents conflicts.
- **go:embed requires static/ to be non-empty** — `static/.gitkeep` satisfies this locally; Docker/CI copies `ui/dist/` in before building.
- **Agent tokens are pre-shared** — generated on the server, manually copied to the agent. No auto-enrollment.
- **WebSocket keepalive** — agent sends ping every 30 s; server closes connection after 60 s without pong; agent reconnects with exponential backoff (1 s → 2 s → 4 s → max 60 s).

## Module interface (reference)

```go
type Module interface {
    ID()      string   // stable, lowercase, no spaces — e.g. "dns"
    Name()    string   // human label for sidebar — e.g. "DNS Blocker"
    Version() string   // module's own semver
    Icon()    string   // SVG string for sidebar icon

    Migrate(db *sql.DB) error              // CREATE TABLE IF NOT EXISTS with <id>_ prefix
    Init(p Platform) error                 // register routes; must not block
    Start(ctx context.Context) error       // background work; return on ctx cancel
    AgentCapable() bool
    AgentStart(ctx context.Context, conn AgentConn) error
}

type Platform interface {
    DB()      *sql.DB
    Mux()     *http.ServeMux
    Auth()    func(http.Handler) http.Handler
    SSE()     SSEBroker
    Config()  ModuleConfig    // reads BIGLYBIGLY_<ID>_* env vars
    Log()     *slog.Logger
}
```

## Agent protocol message shapes (reference)

```json
// Agent → Server (stats)
{ "type": "stats", "module": "dns", "agent": "office-london",
  "ts": "2026-04-29T10:00:00Z", "data": { } }

// Server → Agent (config update)
{ "type": "config", "module": "dns", "data": { } }

// Server → Agent (handshake)
{ "type": "hello", "server_version": "1.0.0", "modules": ["dns"] }
```

## Platform-level REST API

| Method | Path | Description |
|---|---|---|
| GET | `/api/modules` | Loaded modules (id, name, version, icon) |
| GET | `/api/agents` | Connected agents + status |
| GET | `/api/agents/tokens` | List agent tokens |
| POST | `/api/agents/token` | Create agent token |
| DELETE | `/api/agents/token/{id}` | Revoke agent token |
| GET | `/api/auth/providers` | Available OAuth providers |
| POST | `/api/auth/register` | Email/password registration |
| POST | `/api/auth/login` | Email/password login |
| POST | `/api/auth/logout` | End session |
| GET | `/api/events` | SSE stream |
| WS | `/api/agent/connect` | Agent WebSocket |

## Adding features

- **New tool** → create `internal/tools/<id>/module.go`, implement `Module`, add one line to `main.go`. See CONTRIBUTING.md.
- **New platform API endpoint** → add handler in `internal/core/api/server.go`.
- **New auth provider** → add OAuth flow in `internal/core/auth/oauth.go`.
- **New agent message type** → add to `internal/core/agent/protocol.go`, handle in both `server.go` and `client.go`.
- **New UI shell section** → add to `ui/src/components/Shell.tsx`; module pages go in `ui/src/tools/<id>/`.
