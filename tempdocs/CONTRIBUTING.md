# Contributing to Biglybigly

## Ways to contribute

- **Build a new module** — the most impactful contribution; see below
- **Improve an existing module** — bug fixes, new features, better performance
- **Platform improvements** — auth, agent protocol, UI shell, storage
- **Documentation** — corrections, examples, better explanations

---

## Development setup

**Prerequisites:** Go 1.22+, Node 22+, npm

```bash
git clone https://github.com/hamed0406/biglybigly
cd biglybigly

# Build and run (development)
cd ui && npm install && npm run dev &     # UI dev server on :5173
cd ..
go run ./cmd/biglybigly                   # API server on :8080
# UI dev server proxies /api → :8080 automatically
```

**Tests:**

```bash
go test ./...
cd ui && npm run build    # type-checks via tsc
```

**Lint:**

```bash
go vet ./...
cd ui && npm run lint
```

---

## Building a new module

Read [ARCHITECTURE.md](./ARCHITECTURE.md) fully before starting. The short version:

### 1. Create the module package

```
internal/tools/mymodule/
  module.go     — implements platform.Module
  handlers.go   — HTTP handlers
  store.go      — DB queries
  (add files as needed)
```

### 2. Implement the Module interface

```go
package mymodule

import (
    "context"
    "database/sql"
    "net/http"

    "github.com/biglybigly/biglybigly/internal/platform"
)

type Module struct {
    p platform.Platform
}

func New() *Module { return &Module{} }

func (m *Module) ID()      string { return "mymodule" }
func (m *Module) Name()    string { return "My Module" }
func (m *Module) Version() string { return "0.1.0" }
func (m *Module) Icon()    string { return `<svg>...</svg>` }

func (m *Module) Migrate(db *sql.DB) error {
    // use "mymodule_" prefix on all table names
    _, err := db.Exec(`CREATE TABLE IF NOT EXISTS mymodule_items (
        id      INTEGER PRIMARY KEY,
        name    TEXT NOT NULL,
        created INTEGER NOT NULL
    )`)
    return err
}

func (m *Module) Init(p platform.Platform) error {
    m.p = p
    mux := p.Mux()
    auth := p.Auth()
    mux.Handle("GET /api/mymodule/items",  auth(http.HandlerFunc(m.handleList)))
    mux.Handle("POST /api/mymodule/items", auth(http.HandlerFunc(m.handleCreate)))
    return nil
}

func (m *Module) Start(ctx context.Context) error {
    // background work here — return when ctx is cancelled
    <-ctx.Done()
    return nil
}

func (m *Module) AgentCapable() bool { return false }

func (m *Module) AgentStart(ctx context.Context, conn platform.AgentConn) error {
    return nil // not used since AgentCapable() == false
}
```

### 3. Register it in main.go

```go
// cmd/biglybigly/main.go
import "github.com/biglybigly/biglybigly/internal/tools/mymodule"

modules := []platform.Module{
    // existing modules...
    mymodule.New(),   // add this line
}
```

### 4. Create the UI page

```
ui/src/tools/mymodule/
  MyModulePage.tsx   — main page component
  api.ts             — typed fetch wrappers for /api/mymodule/*
```

The shell auto-routes `/tools/mymodule` to your page component.

### 5. Write tests

Every module should have at minimum:
- A test for each DB migration (runs migration, checks schema)
- A test for each HTTP handler (uses `httptest`)
- A test for any non-trivial business logic

### Module checklist

- [ ] `ID()` is lowercase, no spaces, no hyphens — will be used as DB prefix and URL path
- [ ] All DB tables use `<id>_` prefix
- [ ] All API routes live under `/api/<id>/`
- [ ] `Migrate()` uses `CREATE TABLE IF NOT EXISTS` (idempotent)
- [ ] `Start()` returns when `ctx` is cancelled
- [ ] `AgentStart()` is implemented if `AgentCapable()` is true
- [ ] UI page is in `ui/src/tools/<id>/`
- [ ] Tests cover migrations and handlers
- [ ] Module is documented in its own `README.md` inside `internal/tools/<id>/`

---

## Platform changes

Changes to the platform core (`internal/platform/`, `internal/core/`) affect all modules. For these:

1. Open an issue first to discuss the change before writing code
2. Ensure the `Module` interface remains backward compatible — adding methods is a breaking change for all existing modules
3. If the interface must change, bump the platform's major version and update all modules

---

## Pull request guidelines

- One PR per module or per coherent platform change
- Include tests
- Run `go vet ./...` and `go test ./...` before opening the PR
- Describe what the module does and why in the PR description
- For new modules, include a screenshot of the UI page

---

## Code style

- Go: standard `gofmt` formatting; `go vet` must pass
- No third-party linters required beyond `go vet`
- TypeScript: project ESLint config; run `npm run lint`
- Comments: explain *why*, not *what* — well-named identifiers document the what
- No AI-generated boilerplate comments (e.g. "This function handles X by doing Y")

---

## Commit messages

```
feat(dns): add per-device query breakdown
fix(agent): reconnect after server restart
docs: update module checklist in CONTRIBUTING
refactor(platform): extract AgentConn into its own file
```

Format: `<type>(<scope>): <short description>`
Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`
