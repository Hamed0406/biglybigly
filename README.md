# Biglybigly

A self-hosted network management platform built around pluggable modules. Single binary, runs everywhere.

[![CI](https://github.com/Hamed0406/biglybigly/actions/workflows/ci.yml/badge.svg)](https://github.com/Hamed0406/biglybigly/actions/workflows/ci.yml)

## Features

- **🏠 Home Dashboard** — Aggregated overview of all agents, DNS stats, network flows, and alerts at a glance
- **🛡️ DNS Filter** — Pi-hole-style DNS filtering with hosts-file blocklists, custom rules, query log, and per-agent stats
- **🌐 Network Monitor** — Passive monitoring of all outgoing TCP/UDP connections with hostname resolution, process identification, and visual network map
- **📊 System Monitor** — Per-agent CPU, memory, uptime, and hostname tracking with history
- **🔍 URL Monitor** — Track website availability, response times, and status history
- **🤖 Agent Protocol** — Deploy agents on remote hosts; data flows back to a central server via HTTP
- **🔌 Pluggable Modules** — Add new tools by implementing a single Go interface
- **🔋 Single Binary** — Go backend + React UI compiled into one executable
- **🌍 Cross-Platform** — Linux, macOS, Windows (amd64 + arm64)
- **💾 SQLite Database** — Zero-config, no external database needed (pure Go via `modernc.org/sqlite`, no CGO)
- **🐳 Docker Support** — Multi-platform Docker image included

## Quick Start

### Docker (Recommended for Server)

```bash
docker compose up -d
# Visit http://localhost:8082
```

### Binary

```bash
# Download from releases or build from source
./biglybigly
# Visit http://localhost:8082
```

### From Source

```bash
# Prerequisites: Go 1.24+, Node 22+

# Build UI and copy into Go embed directory
cd ui && npm install && npm run build && cd ..
rm -rf internal/core/api/static && cp -r ui/dist internal/core/api/static

# Build binary
go build -o dist/biglybigly ./cmd/biglybigly

# Run
./dist/biglybigly
```

## Modes

The same binary runs as either a **server** or an **agent** — only the environment changes.

### Server (Default)

Hosts the HTTP API + UI, owns the SQLite database, and accepts data from connected agents.

```bash
./biglybigly
# or
BIGLYBIGLY_MODE=server ./biglybigly
```

### Agent

Runs on a remote host, collects local data (network flows, system metrics, DNS queries), and forwards it to the server.

```bash
BIGLYBIGLY_MODE=agent \
BIGLYBIGLY_SERVER_URL=http://your-server:8082 \
BIGLYBIGLY_AGENT_TOKEN=your-token \
BIGLYBIGLY_AGENT_NAME=my-laptop \
./biglybigly
```

The agent must run with **administrator/root** privileges to bind port 53 (DNS filter) and to read connection state on Windows.

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|---|---|---|
| `BIGLYBIGLY_MODE` | `server` | `server` or `agent` |
| `BIGLYBIGLY_HTTP_ADDR` | `:8082` | HTTP listen address (server mode) |
| `BIGLYBIGLY_DB_PATH` | `./biglybigly.db` | SQLite database path |
| `BIGLYBIGLY_BASE_URL` | `http://localhost:8082` | Public URL (for OAuth redirects) |
| `BIGLYBIGLY_SERVER_URL` | — | Server URL (agent mode only) |
| `BIGLYBIGLY_AGENT_TOKEN` | — | Agent auth token (agent mode only) |
| `BIGLYBIGLY_AGENT_NAME` | hostname | Agent identifier sent to server |

## Modules

### 🛡️ DNS Filter (`dnsfilter`)

Pi-hole-style DNS filtering. Acts as a recursive resolver on `127.0.0.1:53` and intercepts every DNS query before it leaves the host.

- **Blocklists** — Auto-downloads hosts-file format lists (Steven Black list pre-configured); refreshed every 6 hours
- **Custom rules** — Block or allow specific domains via the UI; users can paste full URLs (`https://www.bbc.com/persian`) and the domain is auto-extracted
- **Parent-domain matching** — Blocking `bbc.com` also blocks `www.bbc.com` and any subdomain
- **Auto DNS configuration** — Agent automatically sets the system DNS to `127.0.0.1` on startup and restores the original DNS on shutdown
- **VPN/proxy detection** — Warns the user if a VPN or proxy is active that may bypass the filter, with platform-specific remediation hints
- **Query log** — Every DNS query is logged with type, client, and block decision; synced to the server every 30 seconds
- **Rule sync** — Agents poll the server every 5 minutes for blocklist and custom-rule changes

### 🌐 Network Monitor (`netmon`)

Passive monitoring of all outgoing TCP/UDP connections.

- **Linux:** reads `/proc/net/tcp`, `/proc/net/tcp6`, `/proc/net/udp`, `/proc/net/udp6`
- **Windows:** `Get-NetTCPConnection` + `Get-NetUDPEndpoint` (with `netstat` fallback when PowerShell returns 0)
- **macOS:** `lsof -i -n -P`
- Reverse DNS resolution and best-effort process identification
- Deduplication by `(agent, proto, remote_ip, remote_port)` with connection counters
- Searchable UI with **top hosts**, **top ports**, and a visual **network map**

### 📊 System Monitor (`sysmon`)

Per-agent CPU, memory, uptime, and hostname tracking.

- Snapshots every 30 seconds
- Hostname history (detects when a host is renamed)
- Used by the dashboard to determine which agents are online

### 🔍 URL Monitor (`urlcheck`)

Monitors website availability and response times via HTTP HEAD requests.

- Manual or scheduled health checks
- Status code and response time tracking
- Check history (last 100 per URL)

## Home Dashboard

When no module is selected (the default landing page), the dashboard shows:

- **Overview cards** — Agents online / total, DNS queries (24h), DNS blocked (24h), network flows (24h)
- **Per-agent health** — Live mini gauges for CPU and memory, uptime, OS, last-seen
- **Recent blocks** — Last 10 blocked DNS queries
- **Top blocked / top queried domains** — Bar chart of the busiest domains over 24h
- **URL alerts** — Any URL monitor reporting non-200
- Auto-refreshes every 10 seconds

## Logging

Both server and agent write to **stderr** AND a **`biglybigly.log`** file in the working directory (or the directory of `BIGLYBIGLY_DB_PATH`).

Notable INFO-level events:
- Every API request (method, path, remote IP)
- DNS blocks: `DNS BLOCKED domain=... type=... client=...`
- DNS blocks via agent: `DNS BLOCKED (via agent) agent=... domain=...`
- Agent connection status (every 60s)
- Preflight checks at startup (PowerShell, Npcap, admin privileges, etc.)

## Architecture

Biglybigly is a **platform** — modules plug in and get access to:

- Shared SQLite database (with mandatory `<module-id>_` table prefix for isolation)
- HTTP mux with auth middleware
- Structured logging (`log/slog`)
- Agent communication protocol

```
┌─────────────────────────────────────────┐
│              Server                     │
│  ┌──────┐  ┌────────┐  ┌──────────────┐ │
│  │ HTTP │  │   UI   │  │  Modules     │ │
│  │ API  │  │ React  │  │  netmon, …   │ │
│  └──┬───┘  └────────┘  └──────┬───────┘ │
│     │                         │         │
│  ┌──┴─────────────────────────┴───────┐ │
│  │         Platform Core              │ │
│  │  SQLite │ Auth │ Mux │ Log         │ │
│  └────────────────────────────────────┘ │
└────────────────┬────────────────────────┘
                 │ HTTP (ingest)
    ┌────────────┴─────────────┐
    │                          │
┌───┴────────┐         ┌───────┴───────┐
│   Agent    │   …     │   Agent       │
│ (Windows)  │         │ (Raspberry Pi)│
└────────────┘         └───────────────┘
```

For detailed architecture documentation, see [`tempdocs/ARCHITECTURE.md`](tempdocs/ARCHITECTURE.md).

## Building

### Current Platform

```bash
make build
```

### All Platforms

```bash
make build-all
# Outputs to dist/:
#   biglybigly-linux-amd64
#   biglybigly-linux-arm64
#   biglybigly-linux-armv7
#   biglybigly-macos-amd64
#   biglybigly-macos-arm64
#   biglybigly-windows-amd64.exe
#   biglybigly-windows-arm64.exe
```

### Docker

```bash
docker compose build
```

See [`CROSS-PLATFORM.md`](CROSS-PLATFORM.md) for platform-specific notes and systemd setup.

## Development

```bash
# Terminal 1: UI dev server with hot reload
cd ui && npm install && npm run dev

# Terminal 2: Go backend
go run ./cmd/biglybigly
```

The Vite dev server on `:5173` proxies `/api` requests to the Go backend on `:8082`.

### Common Commands

```bash
go build ./...                                # compile
go test ./... -count=1                        # test all
go test ./internal/tools/netmon -run TestX    # single test
go vet ./...                                  # lint Go
cd ui && npm run lint                         # lint TypeScript
cd ui && npm run build                        # build UI for production
```

## Testing

The project has three layers of automated tests:

### 1. Unit tests (Go)

Per-module tests with in-memory SQLite. No external dependencies.

```bash
go test ./... -count=1
```

### 2. Integration tests (Go)

Server-level tests in [`internal/core/api/server_test.go`](internal/core/api/server_test.go) that exercise the HTTP API end-to-end against a real `httptest` recorder + in-memory DB. Covers setup flow, modules endpoint, and the dashboard aggregation.

### 3. End-to-end tests (Playwright)

Browser-based UI tests in [`e2e/`](e2e/) that drive the real React app against a running server.

```bash
cd e2e
npm install
npx playwright install --with-deps    # one-time
npx playwright test
```

Tests cover: dashboard loading, sidebar navigation, DNS filter UI, URL monitor, setup flow, and API smoke tests.

All three test layers run in CI on every push (`.github/workflows/ci.yml`) on Linux, macOS, and Windows.

## Adding a Module

1. Create `internal/tools/<id>/module.go` implementing `platform.Module`
2. Use `<id>_` prefix for all DB tables (e.g. `mymod_things`)
3. Register routes under `/api/<id>/`
4. Create UI in `ui/src/tools/<id>/`
5. Register in `ui/src/App.tsx` `modulePages` map
6. Add one line to `cmd/biglybigly/main.go`:
   ```go
   modules = append(modules, mymodule.New())
   ```
7. Write tests

See [`tempdocs/CONTRIBUTING.md`](tempdocs/CONTRIBUTING.md) for the full checklist and [`internal/tools/urlcheck/`](internal/tools/urlcheck/) for a reference implementation.

## Security

See [`tempdocs/SECURITY.md`](tempdocs/SECURITY.md) for known security findings and recommended fixes.

The DNS filter requires the agent to run with elevated privileges (port 53). The auto DNS-config feature modifies the system's DNS settings — restored automatically on graceful shutdown via `defer`.

## License

See [LICENSE](LICENSE).
