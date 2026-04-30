# Network Monitor Module

Passive network connection monitoring for Biglybigly. Observes all outgoing TCP/UDP connections on the host, resolves hostnames, identifies processes, and provides a searchable dashboard.

## How It Works

The collector polls the OS every 10 seconds (server mode) or 30 seconds (agent mode):

- **Linux**: Reads `/proc/net/tcp`, `/proc/net/tcp6`, `/proc/net/udp`, `/proc/net/udp6`
- **macOS/Windows**: Not yet implemented (planned)

For each connection:
1. Parses source/destination IP and port
2. Filters out loopback and listening-only sockets
3. Attempts reverse DNS lookup for the remote IP
4. Attempts to identify the owning process (best-effort, may need root)
5. Deduplicates by `(agent, proto, remote_ip, remote_port)` — increments a counter on repeated observations

## Limitations

- **Sampled, not exhaustive**: Short-lived connections that open and close between polls will be missed
- **DNS is best-effort**: Hostnames come from reverse DNS lookups, not actual query logs. Some IPs won't resolve.
- **Process identification**: Requires read access to `/proc/*/fd/` (typically root). Non-root agents will show empty process names.
- **Linux-first**: macOS and Windows support is planned but not yet implemented

## API

### GET /api/netmon/flows

List observed connections with optional filtering.

**Query parameters:**
- `search` — Filter by IP, hostname, or process name
- `agent` — Filter by agent name
- `proto` — Filter by protocol (`tcp`, `tcp6`, `udp`, `udp6`)
- `limit` — Max results (default: 200, max: 1000)

**Response:**
```json
[
  {
    "id": 1,
    "agent_name": "local",
    "proto": "tcp",
    "local_ip": "192.168.1.5",
    "local_port": 52341,
    "remote_ip": "142.250.80.46",
    "remote_port": 443,
    "hostname": "lhr25s34-in-f14.1e100.net",
    "pid": 1234,
    "process": "chrome",
    "state": "ESTABLISHED",
    "count": 42,
    "first_seen": 1704067200,
    "last_seen": 1704070800
  }
]
```

### GET /api/netmon/top-hosts

Top remote hosts by total connection count.

**Query parameters:**
- `limit` — Max results (default: 20, max: 100)

**Response:**
```json
[
  { "name": "google.com", "count": 150 },
  { "name": "github.com", "count": 89 }
]
```

### GET /api/netmon/top-ports

Top remote ports by total connection count. Known ports are labeled (e.g., "HTTPS (443)").

**Response:**
```json
[
  { "name": "HTTPS (443)", "count": 320 },
  { "name": "DNS (53)", "count": 45 }
]
```

### GET /api/netmon/stats

Summary statistics.

**Response:**
```json
{
  "total_flows": 156,
  "total_hosts": 42,
  "active_now": 23,
  "unique_agents": 2
}
```

### POST /api/netmon/ingest

Receive flow data from remote agents via HTTP (alternative to WebSocket).

**Request:**
```json
{
  "agent": "office-london",
  "flows": [
    {
      "proto": "tcp",
      "remote_ip": "142.250.80.46",
      "remote_port": 443,
      "hostname": "google.com",
      "state": "ESTABLISHED",
      "process": "chrome",
      "seen_at": 1704067200
    }
  ]
}
```

**Response:**
```json
{ "ingested": 1 }
```

## Database Schema

```sql
CREATE TABLE netmon_flows (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  agent_name  TEXT NOT NULL DEFAULT 'local',
  proto       TEXT NOT NULL,
  local_ip    TEXT,
  local_port  INTEGER,
  remote_ip   TEXT NOT NULL,
  remote_port INTEGER NOT NULL,
  hostname    TEXT,
  pid         INTEGER,
  process     TEXT,
  state       TEXT,
  count       INTEGER NOT NULL DEFAULT 1,
  first_seen  INTEGER NOT NULL,
  last_seen   INTEGER NOT NULL,
  UNIQUE(agent_name, proto, remote_ip, remote_port)
);

CREATE TABLE netmon_dns_cache (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  ip          TEXT NOT NULL UNIQUE,
  hostname    TEXT,
  resolved_at INTEGER NOT NULL
);
```

## Future Enhancements

- [ ] macOS and Windows collectors
- [ ] Configurable poll interval
- [ ] DNS blocklist creation from observed domains
- [ ] Alerting on new/suspicious connections
- [ ] Data retention and automatic cleanup
- [ ] Per-agent configuration from server
- [ ] Connection geo-IP enrichment
- [ ] Export to CSV/JSON
