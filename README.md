# Biglybigly

A self-hosted network management platform built around pluggable modules. Single binary, runs everywhere.

## Features

- **Network Monitor** — Passive monitoring of all outgoing connections (TCP/UDP), with hostname resolution, process identification, and searchable UI
- **URL Monitor** — Track website availability, response times, and status history
- **Pluggable Modules** — Add new tools by implementing a single Go interface
- **Agent Protocol** — Deploy agents on remote hosts to collect and forward data to a central server
- **Single Binary** — Go backend + React UI compiled into one executable
- **Cross-Platform** — Linux, macOS, Windows (amd64 + arm64)
- **SQLite Database** — Zero-config, no external database needed
- **Docker Support** — Multi-platform Docker image included

## Quick Start

### Binary

```bash
# Download from releases or build from source
./biglybigly

# Visit http://localhost:8082
```

### Docker

```bash
docker compose up -d

# Visit http://localhost:8082
```

### From Source

```bash
# Prerequisites: Go 1.22+, Node 22+

# Build UI
cd ui && npm install && npm run build && cd ..

# Copy UI into Go embed directory
cp -r ui/dist/* internal/core/api/static/

# Build binary
go build -o dist/biglybigly ./cmd/biglybigly

# Run
./dist/biglybigly
```

## Modes

### Server (Default)

Runs the HTTP server, UI, database, and accepts agent connections.

```bash
./biglybigly
# or
BIGLYBIGLY_MODE=server ./biglybigly
```

### Agent

Runs on remote hosts, collects data, and sends it to a central server via WebSocket.

```bash
BIGLYBIGLY_MODE=agent \
BIGLYBIGLY_SERVER_URL=http://your-server:8082 \
BIGLYBIGLY_AGENT_TOKEN=your-token \
./biglybigly
```

Same binary for both modes — just change the environment variable.

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|---|---|---|
| `BIGLYBIGLY_MODE` | `server` | `server` or `agent` |
| `BIGLYBIGLY_HTTP_ADDR` | `:8082` | HTTP listen address |
| `BIGLYBIGLY_DB_PATH` | `./biglybigly.db` | SQLite database path |
| `BIGLYBIGLY_BASE_URL` | `http://localhost:8082` | Public URL (for OAuth redirects) |
| `BIGLYBIGLY_SERVER_URL` | — | Server URL (agent mode only) |
| `BIGLYBIGLY_AGENT_TOKEN` | — | Agent auth token (agent mode only) |

See [`.env.example`](.env.example) for all options including OAuth configuration.

## Modules

### Network Monitor (`netmon`)

Passively monitors all outgoing TCP/UDP connections on the host.

- Reads `/proc/net/tcp`, `/proc/net/tcp6`, `/proc/net/udp`, `/proc/net/udp6` (Linux)
- Reverse DNS resolution for remote IPs
- Process identification (best-effort, may need elevated permissions)
- Deduplication with connection counts
- Searchable UI with top hosts and top ports views
- Auto-refresh dashboard

### URL Monitor (`urlcheck`)

Monitors website availability and response times.

- Add URLs to monitor
- Manual health checks (HTTP HEAD requests)
- Status code and response time tracking
- Check history (last 100 per URL)

See [`internal/tools/urlcheck/README.md`](internal/tools/urlcheck/README.md) for API documentation.

## Architecture

Biglybigly is a **platform** — modules plug in and get access to:
- Shared SQLite database
- HTTP mux with auth middleware
- SSE event broker
- Structured logging
- Agent communication protocol

```
┌─────────────────────────────────────────┐
│              Server                      │
│  ┌──────┐  ┌──────────┐  ┌───────────┐ │
│  │ HTTP │  │  Module   │  │  Module   │ │
│  │Server│  │ netmon    │  │ urlcheck  │ │
│  └──┬───┘  └────┬─────┘  └─────┬─────┘ │
│     │           │               │       │
│  ┌──┴───────────┴───────────────┴─────┐ │
│  │         Platform Core              │ │
│  │  SQLite │ Auth │ SSE │ Mux │ Log   │ │
│  └──────────────────────────────┬─────┘ │
│                                 │       │
│  ┌──────────────────────────────┴─────┐ │
│  │       WebSocket Agent Server       │ │
│  └──────────────────────────────┬─────┘ │
└─────────────────────────────────┼───────┘
                                  │
            ┌─────────────────────┼──────────────┐
            │                     │              │
     ┌──────┴───────┐    ┌───────┴──────┐  ┌────┴───────┐
     │   Agent A    │    │   Agent B    │  │   Agent C  │
     │ (office-lon) │    │ (home-srv)   │  │ (rpi-lab)  │
     └──────────────┘    └──────────────┘  └────────────┘
```

For detailed architecture documentation, see [`ARCHITECTURE.md`](tempdocs/ARCHITECTURE.md).

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

See [`CROSS-PLATFORM.md`](CROSS-PLATFORM.md) for platform-specific notes and systemd service setup.

## Development

```bash
# Terminal 1: UI dev server with hot reload
cd ui && npm install && npm run dev

# Terminal 2: Go backend
go run ./cmd/biglybigly
```

The Vite dev server on `:5173` proxies `/api` requests to the Go backend on `:8082`.

### Commands

```bash
go build ./...                              # compile
go test ./...                               # test all
go test ./internal/tools/netmon -run TestX   # single test
go vet ./...                                # lint Go
cd ui && npm run lint                        # lint TypeScript
```

## Adding a Module

1. Create `internal/tools/<id>/module.go` implementing `platform.Module`
2. Use `<id>_` prefix for all DB tables
3. Register routes under `/api/<id>/`
4. Create UI in `ui/src/tools/<id>/`
5. Add one line to `cmd/biglybigly/main.go`:
   ```go
   modules = append(modules, mymodule.New())
   ```
6. Write tests

See [`CONTRIBUTING.md`](tempdocs/CONTRIBUTING.md) for the full checklist and [`internal/tools/urlcheck/`](internal/tools/urlcheck/) for a reference implementation.

## Security

See [`SECURITY.md`](tempdocs/SECURITY.md) for known security findings and recommended fixes.

## License

See [LICENSE](LICENSE).
