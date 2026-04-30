# Security Findings — Biglybigly

This document records security and reliability issues identified in an architectural review of the platform core. Issues are grouped by severity. Each entry includes the affected file and line, a description of the risk, and the recommended fix.

---

## Critical

### C1 — Unauthenticated `/api/modules` and `/api/agents`

**File:** `cmd/biglybigly/main.go:83,95`

`GET /api/modules` and `GET /api/agents` are registered without auth middleware. Any unauthenticated caller who can reach port 8080 can enumerate all loaded module IDs and names, and all connected agent names and connection timestamps. This leaks the full deployment topology.

**Fix:** Wrap both handlers with `requireAuth`:

```go
mux.Handle("GET /api/modules", authStore.RequireAuth(registry))
```

Update `App.tsx` to redirect to the login page when `/api/modules` returns 401.

---

### C2 — Open user registration

**File:** `internal/core/api/server.go:42`

`POST /api/auth/register` is unauthenticated with no per-IP limit, no invite requirement, no first-user bootstrap, and no password policy. Any caller who can reach the server can create an account. All accounts have identical access — there is no admin role.

**Fix:** Add a first-user-wins gate: if the `users` table is empty, allow registration; once any user exists, require an admin invitation or set `BIGLYBIGLY_OPEN_REGISTRATION=true` to re-enable. Add a minimum password length check (8 characters) in `auth.Register()`.

---

### C3 — Session cookie missing `Secure` flag

**Files:** `internal/core/auth/auth.go:161`, `internal/core/api/server.go:161`

Both `sessionCookie()` helpers set `HttpOnly: true` and `SameSite: Lax` but not `Secure: true`. Session tokens are transmitted in cleartext over HTTP when the server is accessed without TLS.

**Fix:**

```go
Secure: strings.HasPrefix(cfg.BaseURL, "https://"),
```

Thread `cfg.BaseURL` to both helpers, or consolidate the two helpers into one (see M4).

---

### C4 — No rate limiting on authentication endpoints

**File:** `internal/core/api/server.go`

`POST /api/auth/login` and `POST /api/auth/register` have no rate limiting. bcrypt slows individual attempts but does not prevent parallel distributed attacks.

**Fix:** Add a per-IP in-memory counter on both endpoints. 10 attempts per IP per 5 minutes is a reasonable starting point for a self-hosted platform.

---

## High

### H1 — WebSocket `CheckOrigin` accepts all origins

**File:** `internal/core/agent/server.go:22`

```go
CheckOrigin: func(r *http.Request) bool { return true }
```

Any origin can initiate a WebSocket upgrade to `/api/agent/connect`. Agents authenticate with a Bearer token immediately after upgrade, but the upgrade itself is unrestricted.

**Fix:** Document explicitly why this is intentional (agent clients are not browsers), or restrict to `Origin: <cfg.BaseURL>` for browser-initiated connections.

---

### H2 — `http.DefaultClient` with no timeout in OAuth flows

**File:** `internal/core/auth/oauth.go:89,145,159`

`http.DefaultClient.Do()` has no timeout. A slow or hung response from Google's or GitHub's endpoints will block an HTTP handler goroutine indefinitely. Under load, this exhausts the goroutine pool.

**Fix:**

```go
var oauthHTTPClient = &http.Client{Timeout: 5 * time.Second}
```

Use `oauthHTTPClient` in place of `http.DefaultClient` throughout `oauth.go`.

---

### H3 — OAuth error handling silently swallowed

**File:** `internal/core/auth/oauth.go:34,35,103,104`

`randomHex` and `s.db.Exec` errors are discarded with `_`. If the CSPRNG fails, `state` is an empty string and the flow continues. If the state insert fails, the callback validation will produce a misleading "invalid state" error with no logging.

**Fix:** Check both errors and return a 500 response on failure:

```go
state, err := randomHex(16)
if err != nil {
    http.Error(w, "internal error", http.StatusInternalServerError)
    return
}
if _, err := s.db.Exec(...); err != nil {
    http.Error(w, "internal error", http.StatusInternalServerError)
    return
}
```

---

### H4 — OAuth state records never expire

**File:** `internal/core/storage/migrations.go:29`

The `oauth_states` table stores state tokens with `created_at` but nothing prunes stale rows. State tokens accumulate indefinitely.

**Fix:** Add a TTL check in both OAuth callbacks (`AND created_at > unixepoch() - 600`) and add a background cleanup goroutine that runs `DELETE FROM oauth_states WHERE created_at < unixepoch() - 600` every 15 minutes.

---

### H5 — Google OAuth does not verify `email_verified`

**File:** `internal/core/auth/oauth.go:93–98`

The Google userinfo response is decoded without checking `email_verified: true`. Google can return unverified email addresses for some account types. An attacker could register an unverified email that belongs to another user and gain access to their account when that user later authenticates.

**Fix:**

```go
if !userInfo.EmailVerified {
    http.Error(w, "email not verified", http.StatusForbidden)
    return
}
```

---

### H6 — Agent name collision silently evicts connected agents

**File:** `internal/core/agent/server.go:84`

`s.agents[name] = ag` overwrites any existing entry. A second agent connecting with the same token silently takes over the map slot. The first agent's goroutine continues running but is no longer reachable. An adversary who steals a token can repeatedly connect to evict the legitimate agent.

**Fix:** Key the agents map on the token's integer primary key (pass the ID from `validateToken`) rather than the name, or append a random per-connection suffix. Log a warning whenever a name collision occurs.

---

## Medium

### M1 — `SessionSecret` config field is loaded but never used

**File:** `internal/core/config/config.go:55`

`BIGLYBIGLY_SESSION_SECRET` is loaded and stored in `Config.SessionSecret` but no code reads it. Session security comes from random token generation, not HMAC signing. The field misleads operators into thinking that setting it provides a security benefit.

**Fix:** Remove the field from `Config` and from `.env.example`, or implement HMAC-signed cookies and fail startup if the secret equals the default value `"change-me-in-production"`.

---

### M2 — No HTTP server-level timeouts

**File:** `cmd/biglybigly/main.go:116`

```go
srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}
```

No `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, or `IdleTimeout` is set. Slow clients can hold connections open indefinitely.

**Fix:**

```go
srv := &http.Server{
    Addr:              cfg.HTTPAddr,
    Handler:           mux,
    ReadHeaderTimeout: 10 * time.Second,
    IdleTimeout:       120 * time.Second,
}
```

For SSE connections, `WriteTimeout` must be 0 or very large — use `http.ResponseController` to override the timeout per-response.

---

### M3 — No request body size limits

**File:** `internal/core/api/server.go:72,95,135`

All POST handlers call `json.NewDecoder(r.Body).Decode(...)` with no `http.MaxBytesReader` wrapper. A client can POST an arbitrarily large body and force the server to read it into memory.

**Fix:** Prepend to each POST handler:

```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
```

---

### M4 — Duplicate `sessionCookie` helper with divergent TTL

**Files:** `internal/core/auth/auth.go:161`, `internal/core/api/server.go:161`

Two nearly identical helpers exist. `auth.go` computes `MaxAge` from the `sessionTTL` constant; `api/server.go` hardcodes `2592000`. If `sessionTTL` ever changes, the email/password path silently diverges.

**Fix:** Delete `sessionCookieFromToken` in `api/server.go`. Export the helper from `auth/auth.go` and call it from `api/server.go`.

---

### M5 — Agent reconnect backoff never resets

**File:** `internal/core/agent/client.go:40–50`

`backoff` starts at 1 s and doubles up to 60 s on each iteration but is never reset after a successful connection. After a long-running agent disconnects, the next reconnect waits the accumulated backoff (up to 60 s) rather than starting fresh.

**Fix:** Reset `backoff = time.Second` at the top of the loop body (after each iteration begins), or reset it inside `connect()` on a successful connection.

---

### M6 — Graceful shutdown has no deadline

**File:** `cmd/biglybigly/main.go:126`

```go
srv.Shutdown(context.Background())
```

An unbounded context is passed. If any handler or SSE connection does not respect cancellation, shutdown blocks forever, which breaks PID-1 restart semantics under Docker.

**Fix:**

```go
shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
defer shutCancel()
srv.Shutdown(shutCtx)
```

---

### M7 — XSS via module icon SVG in `dangerouslySetInnerHTML`

**File:** `ui/src/components/Shell.tsx:55`

```tsx
dangerouslySetInnerHTML={{ __html: m.icon }}
```

Module icons are compiled into the binary today, so exploitation requires a malicious module to be registered. However, the platform encourages third-party module contributions and provides no sanitization layer between the server's icon string and the DOM. A module with a crafted `Icon()` return value can execute arbitrary JavaScript in every user's browser session.

**Fix:** Run icon strings through a DOM-based SVG sanitizer (e.g. DOMPurify) before passing to `dangerouslySetInnerHTML`, or restrict `Icon()` at the platform level to a known-safe set of SVG elements (`<svg>`, `<path>`, `<circle>`, `<rect>`).

---

## Low

### L1 — Docker container runs as root

**File:** `Dockerfile`

No `USER` directive is present. The container runs as root. A compromised application has root within the container.

**Fix:** Add before `ENTRYPOINT`:

```dockerfile
RUN addgroup -S bigly && adduser -S -G bigly bigly
USER bigly:bigly
```

Ensure the `/data` directory is writable by this UID.

---

### L2 — SQLite file permissions in `docker-compose.yml`

**File:** `docker-compose.yml`

`./data` is mounted with no UID mapping. On a multi-user host the SQLite file (which contains bcrypt hashes and session tokens) is readable by any process running as the same UID as the container.

**Fix:** Add explicit UID mapping or set permissions on `./data` to `700` during first run.

---

### L3 — No CI security scanning

**File:** `.github/workflows/ci.yml`

CI runs only `go vet` and `npm run lint`. No `staticcheck`, `gosec`, or `govulncheck` is run. Dependency vulnerability scanning is absent.

**Fix:** Add to `ci.yml`:

```yaml
- run: go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...
- run: go install honnef.co/go/tools/cmd/staticcheck@latest && staticcheck ./...
```

---

### L4 — OAuth `http.NewRequest` errors silently ignored

**File:** `internal/core/auth/oauth.go:87,137`

```go
req, _ := http.NewRequest("GET", ...)
```

Errors from `http.NewRequest` are discarded. While near-impossible for hardcoded literal URLs, the pattern will suppress real errors if the URL is ever made dynamic.

**Fix:** Check the error and return a 500 response on failure.

---

### L5 — `rows.Scan` error ignored in example module

**File:** `internal/tools/example/module.go:67`

`rows.Scan(...)` error is discarded. This is the reference implementation that new module authors will copy.

**Fix:** Check the error:

```go
if err := rows.Scan(&it.ID, &it.Name, &it.CreatedAt); err != nil {
    return nil, err
}
```

---

## Documentation inaccuracies

The following items were found to be incorrect in `ARCHITECTURE.md` or `CLAUDE.md` at the time of this review. They have been corrected in the respective files.

| Documented | Reality | Fixed in |
|---|---|---|
| `POST /api/auth/exchange` listed as a platform route | Route is not implemented anywhere | `ARCHITECTURE.md`, `CLAUDE.md` |
| `Platform.Devices()` / `DeviceRegistry` referenced as cross-module sharing mechanism | Method does not exist on the `Platform` interface | `ARCHITECTURE.md` |
| `BIGLYBIGLY_SESSION_SECRET` described as a security setting | Field is loaded but never consumed by any code | `ARCHITECTURE.md` (see M1) |
| Dockerfile described as "4-stage" | Dockerfile has 3 stages (Node UI build, Go binary build, runtime) | `ARCHITECTURE.md` |
| `GET /api/agents/token` listed in route table | Actual route is `GET /api/agents/tokens` (plural) | `ARCHITECTURE.md`, `CLAUDE.md` |

---

## Recommended fix order

1. Session cookie `Secure` flag (C3) — 2-line change, zero risk
2. Authenticate `GET /api/modules` and `GET /api/agents` (C1)
3. HTTP server timeouts and body size limits (M2, M3)
4. Rate limiting on login and register (C4)
5. First-user registration gate and password minimum length (C2)
6. OAuth hardening — `email_verified`, state TTL, error handling, HTTP client timeout (H3, H4, H5, H2)
7. Agent name collision fix (H6)
8. Graceful shutdown deadline (M6)
9. Consolidate `sessionCookie` helper (M4)
10. Agent backoff reset (M5)
11. SVG icon sanitization (M7)
12. Non-root Docker user (L1)
13. CI security scanning (L3)
14. Remove dead `SessionSecret` config field (M1)
