# URL Monitor Module

Simple URL health check and monitoring tool. Add URLs you want to monitor, and the module tracks their status and response times.

## Features

- ✓ Add and remove URLs to monitor
- ✓ Manual health checks (HTTP HEAD requests)
- ✓ Status code tracking (200, 404, 5xx, etc.)
- ✓ Response time measurement (in milliseconds)
- ✓ Check history (last 100 checks per URL)
- ✓ Clean UI with status indicators

## API

### GET /api/urlcheck/urls
List all monitored URLs.

**Response:**
```json
[
  {
    "id": 1,
    "url": "https://example.com",
    "status": 200,
    "last_check": 1704067200,
    "created_at": 1704067200,
    "updated_at": 1704067200
  }
]
```

### POST /api/urlcheck/urls
Add a new URL to monitor.

**Request:**
```json
{
  "url": "https://example.com"
}
```

**Response:**
```json
{
  "id": 1
}
```

### DELETE /api/urlcheck/urls/{id}
Remove a URL from monitoring.

### GET /api/urlcheck/check/{id}
Manually check a URL's status (HTTP HEAD request).

**Response:**
```json
{
  "status": 200,
  "response_time": 145,
  "error": ""
}
```

### GET /api/urlcheck/history/{id}
Get check history for a URL (last 100 checks).

**Response:**
```json
[
  {
    "id": 1,
    "status": 200,
    "response_time": 145,
    "error": "",
    "checked_at": 1704067200
  }
]
```

## Database Schema

```sql
-- URLs to monitor
CREATE TABLE urlcheck_urls (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  url         TEXT NOT NULL UNIQUE,
  status      INTEGER,                    -- last status code
  last_check  INTEGER,                    -- unix timestamp
  created_at  INTEGER NOT NULL,           -- unix timestamp
  updated_at  INTEGER NOT NULL            -- unix timestamp
);

-- History of checks
CREATE TABLE urlcheck_history (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  url_id      INTEGER NOT NULL,           -- foreign key
  status      INTEGER NOT NULL,
  response_time INTEGER,                  -- milliseconds
  error       TEXT,                       -- error message if check failed
  checked_at  INTEGER NOT NULL            -- unix timestamp
);
```

## Future Enhancements

- [ ] Automatic periodic checks (background job)
- [ ] Downtime alerts (email/webhook)
- [ ] Custom check intervals per URL
- [ ] Agent support (remote monitoring)
- [ ] Uptime statistics (% uptime, avg response time)
- [ ] Custom headers for authenticated endpoints
- [ ] POST/PUT request support (not just HEAD)
- [ ] Status page view (all URLs at a glance)
