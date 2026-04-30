# Copilot Instructions for Biglybigly

## Project Overview

**Biglybigly** is a self-hosted network management platform built around pluggable modules (tools). The platform provides:
- Authentication (email/password + OAuth via Google/GitHub)
- Shared SQLite database (no CGO — pure Go using `modernc.org/sqlite`)
- HTTP server with React UI shell
- WebSocket-based agent protocol for remote execution

The platform ships as a single binary with all modules compiled in. It runs in two modes:
- **server** (default) — full UI, database, accepts remote agents
- **agent** — no UI, connects to a server, runs module logic remotely

## Build & Test Commands

### Go (run from repo root)
```bash
go build ./...          # compile all packages
go test ./...           # run all tests
go test ./... -v        # verbose output
go test ./tools/example -run TestMigrate  # run single test
go vet ./...            # lint with go vet
```

### UI (run from `ui/` directory)
```bash
npm install             # install dependencies
npm run dev             # Vite dev server on :5173, proxies /api → :8080
npm run build           # build for production (output: dist/)
npm run lint            # ESLint
```

### Docker
```bash
docker compose build    # build image
docker compose up -d    # start container
```

### Development Workflow
```bash
# Terminal 1: UI dev server
cd ui && npm install && npm run dev

# Terminal 2: API server (watches for code changes if using `go run`)
cd .. && go run ./cmd/biglybigly
# UI dev server automatically proxies /api → :8080
```

### Agent Mode
```bash
BIGLYBIGLY_MODE=agent \
BIGLYBIGLY_SERVER_URL=https://example.com \
BIGLYBIGLY_AGENT_TOKEN=abc123 \
go run ./cmd/biglybigly
```

## Directory Layout

```
cmd/
  biglybigly/
    main.go             ← ONLY place where modules are registered

internal/
  platform/
    module.go           ← Module interface contract
    registry.go         ← Module registration & lifecycle
    
  core/
    config/             ← Env var config loading
    storage/            ← SQLite setup & platform migrations
    auth/               ← Sessions, bcrypt, OAuth
    api/                ← HTTP server, routes, SPA serving
    sse/                ← Server-sent events broker
    agent/              ← WebSocket server (accept agents) + client (agent-side)
    
  tools/                ← One directory per module
    example/            ← Reference implementation

ui/
  src/
    App.tsx             ← Builds sidebar from /api/modules → routes to module pages
    components/
      Shell.tsx         ← Top bar + sidebar
      AgentsPage.tsx    ← Agent status
      SettingsPage.tsx  ← Auth tokens, agent tokens
    tools/
      <module-id>/      ← One dir per module's UI
        <ModuleName>Page.tsx
        api.ts          ← Typed fetch wrappers
    types.ts            ← TS mirrors of Go JSON structs
```

## Key Architectural Constraints

### Database
- **No CGO** — uses `modernc.org/sqlite` (pure Go). Go 1.22+ required.
- **Module table prefix** — all module tables must use `<module-id>_` prefix (e.g., `dns_queries`, `scanner_devices`)
- **Idempotent migrations** — use `CREATE TABLE IF NOT EXISTS`
- **Modules are isolated** — never access another module's tables

### API Routes
- **Module routes** — all module endpoints live under `/api/<module-id>/` (enforced namespacing)
- **Platform routes** — core routes in `/api/modules`, `/api/agents`, `/api/auth/`, `/api/events`, `/api/agent/connect` (WebSocket)
- **Authentication** — wrap handlers with `p.Auth()` middleware; see SECURITY.md for current gaps

### Module Interface
Every module implements `platform.Module`:
```go
ID()          string                                      // stable, lowercase, no spaces
Name()        string                                      // human label
Version()     string                                      // semver
Icon()        string                                      // SVG string
Migrate(db *sql.DB)                          error       // run once, use <id>_ prefix
Init(p Platform)                             error       // register routes, non-blocking
Start(ctx context.Context)                   error       // background work, return on ctx cancel
AgentCapable()                               bool        // if true, implement AgentStart
AgentStart(ctx context.Context, conn AgentConn) error   // agent-side logic
```

### Module Registration
Modules are registered in **one place only**: `cmd/biglybigly/main.go`. Each module must be explicitly added:
```go
modules = append(modules, mymodule.New())  // ← add one line per new module
```

Sidebar entry, routing, and API mounts happen automatically after registration.

### Agent Protocol
Agents communicate via WebSocket at `/api/agent/connect` with Bearer token auth. Messages are JSON:
- **Handshake:** Server sends `{"type": "hello", "modules": [...]}` after upgrade
- **Stats:** Agent sends `{"type": "stats", "module": "dns", "data": {...}}` every 30 s
- **Config:** Server sends `{"type": "config", "module": "dns", "data": {...}}` on updates
- **Keepalive:** Standard WebSocket ping/pong every 30 s; server closes after 60 s without pong

Agent reconnects with exponential backoff: 1 s → 2 s → 4 s → ... → 60 s max.

## Key Conventions

### Module Development
1. Create `internal/tools/<id>/module.go` implementing `platform.Module`
2. Use `<id>_` prefix for all DB tables (e.g., `dns_queries`)
3. Register all routes under `/api/<id>/` with auth middleware if needed
4. Create `ui/src/tools/<id>/` with TypeScript page component
5. Add one line to `cmd/biglybigly/main.go`
6. Write tests for: migrations, HTTP handlers, business logic
7. Include `README.md` in the module directory documenting the module

### Code Style
- **Go:** Follow `gofmt`; `go vet` must pass; comments explain *why*, not *what*
- **TypeScript:** Follow project ESLint config; run `npm run lint` in `ui/`
- **No AI boilerplate** — avoid generated-comment clutter

### Commit Messages
Format: `<type>(<scope>): <description>`

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

Example: `feat(dns): add per-device query breakdown`

## Important Security Notes

Review `SECURITY.md` before submitting changes. Notable issues:
- **C1:** `/api/modules` and `/api/agents` are unauthenticated (topology leak)
- **C2:** Open registration with no password policy
- **C3:** Session cookie missing `Secure` flag
- **C4:** No rate limiting on auth endpoints
- **H6:** Agent name collisions silently evict connected agents
- **M7:** Module icon SVGs are not sanitized (XSS vector)

## UI Conventions

- The sidebar is built dynamically from `/api/modules` at startup
- Each module page lives in `ui/src/tools/<module-id>/` and is lazy-loaded
- API wrappers go in `api.ts` inside each module's directory
- Use shared `ui/src/api/client.ts` for platform-level endpoints
- Types are in `ui/src/types.ts` (TypeScript mirrors of Go JSON structs)

## Common Tasks

### Add a New Module
1. Create `internal/tools/<id>/module.go` implementing `platform.Module`
2. Implement migrations, routes, handlers in separate files as needed
3. Add UI in `ui/src/tools/<id>/`
4. Import and register in `cmd/biglybigly/main.go`
5. Write tests

See `internal/tools/example/` for reference implementation and `CONTRIBUTING.md` for detailed checklist.

### Add a Platform API Route
1. Add handler in `internal/core/api/server.go`
2. Register with `p.Mux().Handle(...)` 
3. Wrap with `p.Auth()` if authentication required
4. Add TypeScript wrapper in `ui/src/api/client.ts`
5. Add tests

### Add a New Auth Provider
1. Add OAuth flow in `internal/core/auth/oauth.go`
2. Handle callback with state validation
3. Create or link user account
4. Set session cookie
5. Redirect to UI

### Agent-Capable Module
1. Set `AgentCapable()` to return `true`
2. Implement `AgentStart(ctx context.Context, conn AgentConn) error`
3. Use `conn.Send(msgType, data)` to push stats
4. Listen on `conn.Receive()` for config updates from server
5. Return cleanly when `ctx` is cancelled
