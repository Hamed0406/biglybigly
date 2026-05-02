// Package agent implements the agent-side runtime of biglybigly.
//
// It provides:
//   - An HTTP Client used to push collected data (network flows, system
//     snapshots, DNS query logs) to the central server and to fetch
//     configuration such as filter rules.
//   - Platform-specific preflight checks (privileges, required tooling such
//     as PowerShell or lsof, packet-capture drivers) that run at startup.
//   - A cross-platform DNSConfigurator that points the host's resolver at
//     the local DNS filter (127.0.0.1) and restores the previous settings
//     on shutdown.
//   - VPN/proxy detection used to warn the operator when a tunnel is
//     likely to bypass the local DNS filter.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ConnStatus represents the current connection state of the agent's link
// to the central server.
type ConnStatus string

// Connection states reported via Client.Status.
const (
	StatusConnected    ConnStatus = "connected"
	StatusDisconnected ConnStatus = "disconnected"
	StatusConnecting   ConnStatus = "connecting"
)

// Client is the agent-side HTTP client that pushes collected data to the
// central biglybigly server and fetches configuration from it. It is safe
// for concurrent use; status and counters are guarded by an internal mutex.
type Client struct {
	serverURL  string
	agentName  string
	agentToken string
	logger     *slog.Logger
	httpClient *http.Client

	mu            sync.RWMutex
	status        ConnStatus
	lastSendAt    time.Time
	lastError     string
	totalSent     int64
	totalErrors   int64
	flowsSent     int64
}

// NewClient returns a Client configured to talk to serverURL, identifying
// itself as agentName and authenticating with agentToken (sent as an
// HTTP Bearer token when non-empty).
func NewClient(serverURL, agentName, agentToken string, logger *slog.Logger) *Client {
	return &Client{
		serverURL:  strings.TrimRight(serverURL, "/"),
		agentName:  agentName,
		agentToken: agentToken,
		logger:     logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		status:     StatusDisconnected,
	}
}

// Status returns the current connection status as last observed by an
// outbound request.
func (c *Client) Status() ConnStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// Stats returns cumulative counters since the Client was created:
// total successful posts, total errors, total flows accepted by the
// server, the time of the last successful send, and the last error
// message (empty if the most recent send succeeded).
func (c *Client) Stats() (totalSent, totalErrors, flowsSent int64, lastSend time.Time, lastErr string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.totalSent, c.totalErrors, c.flowsSent, c.lastSendAt, c.lastError
}

func (c *Client) setStatus(s ConnStatus) {
	c.mu.Lock()
	c.status = s
	c.mu.Unlock()
}

// IngestPayload is the JSON envelope sent to /api/netmon/ingest.
// Flows is left as interface{} so each collector can supply its own
// concrete slice type without an import cycle.
type IngestPayload struct {
	Agent string      `json:"agent"`
	Flows interface{} `json:"flows"`
}

// SendFlows posts a batch of collected network flows to the server's
// netmon ingest endpoint and updates connection status / counters.
func (c *Client) SendFlows(ctx context.Context, flows interface{}) error {
	c.setStatus(StatusConnecting)

	payload := IngestPayload{
		Agent: c.agentName,
		Flows: flows,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	url := fmt.Sprintf("%s/api/netmon/ingest", c.serverURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.agentToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.agentToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.mu.Lock()
		c.status = StatusDisconnected
		c.totalErrors++
		c.lastError = err.Error()
		c.mu.Unlock()
		c.logger.Warn("SendFlows: network error", "url", url, "err", err)
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Capture a bounded slice of the response body so the error
		// surfaced to the caller and logs is useful but not unbounded.
		var errBody []byte
		errBody = make([]byte, 512)
		n, _ := resp.Body.Read(errBody)
		errDetail := string(errBody[:n])

		c.mu.Lock()
		c.status = StatusDisconnected
		c.totalErrors++
		c.lastError = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, errDetail)
		c.mu.Unlock()
		c.logger.Warn("SendFlows: server rejected",
			"status", resp.StatusCode,
			"url", url,
			"response", errDetail,
		)
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var result struct {
		Ingested int `json:"ingested"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	c.mu.Lock()
	c.status = StatusConnected
	c.totalSent++
	c.flowsSent += int64(result.Ingested)
	c.lastSendAt = time.Now()
	c.lastError = ""
	c.mu.Unlock()

	c.logger.Info("Sent flows to server", "ingested", result.Ingested)
	return nil
}

// sendJSON marshals payload as JSON, POSTs it to path on the configured
// server (with the agent's bearer token if set) and updates connection
// status / counters based on the outcome.
func (c *Client) sendJSON(ctx context.Context, path string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	url := fmt.Sprintf("%s%s", c.serverURL, path)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.agentToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.agentToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.mu.Lock()
		c.status = StatusDisconnected
		c.totalErrors++
		c.lastError = err.Error()
		c.mu.Unlock()
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody := make([]byte, 512)
		n, _ := resp.Body.Read(errBody)
		errDetail := string(errBody[:n])
		c.mu.Lock()
		c.totalErrors++
		c.lastError = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, errDetail)
		c.mu.Unlock()
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	c.mu.Lock()
	c.status = StatusConnected
	c.totalSent++
	c.lastSendAt = time.Now()
	c.lastError = ""
	c.mu.Unlock()

	return nil
}

// SendSysmon posts a system-monitoring snapshot to /api/sysmon/ingest.
func (c *Client) SendSysmon(ctx context.Context, snapshot interface{}) error {
	payload := map[string]interface{}{
		"agent":    c.agentName,
		"snapshot": snapshot,
	}
	return c.sendJSON(ctx, "/api/sysmon/ingest", payload)
}

// SendDNSLogs posts a batch of DNS query log entries to
// /api/dnsfilter/ingest.
func (c *Client) SendDNSLogs(ctx context.Context, queries interface{}) error {
	payload := map[string]interface{}{
		"agent":   c.agentName,
		"queries": queries,
	}
	return c.sendJSON(ctx, "/api/dnsfilter/ingest", payload)
}

// FetchJSON performs an authenticated GET against path on the server
// and decodes the JSON response into dest. It is used for pulling
// configuration such as DNS filter rules from the server (see
// SyncRulesFromServer in the dnsfilter module).
func (c *Client) FetchJSON(ctx context.Context, path string, dest interface{}) error {
	url := fmt.Sprintf("%s%s", c.serverURL, path)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if c.agentToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.agentToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(dest)
}

// Ping verifies that the configured server is reachable and that the
// netmon ingest endpoint is mounted. It performs two probes — a GET on
// /api/modules for basic reachability and an empty POST on
// /api/netmon/ingest — so that misconfigurations where the server is up
// but modules failed to register are surfaced clearly to the operator.
func (c *Client) Ping(ctx context.Context) error {
	// First check basic server reachability.
	url := fmt.Sprintf("%s/api/modules", c.serverURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach server at %s: %w", c.serverURL, err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d for /api/modules", resp.StatusCode)
	}
	c.logger.Info("Server reachable", "url", c.serverURL)

	// Now test the ingest endpoint is actually registered. A 404 here
	// usually means the netmon module didn't initialize on the server.
	ingestURL := fmt.Sprintf("%s/api/netmon/ingest", c.serverURL)
	testPayload := []byte(`{"agent":"ping-test","flows":[]}`)
	req2, err := http.NewRequestWithContext(ctx, "POST", ingestURL, bytes.NewReader(testPayload))
	if err != nil {
		return err
	}
	req2.Header.Set("Content-Type", "application/json")
	if c.agentToken != "" {
		req2.Header.Set("Authorization", "Bearer "+c.agentToken)
	}

	resp2, err := c.httpClient.Do(req2)
	if err != nil {
		return fmt.Errorf("ingest endpoint unreachable: %w", err)
	}
	resp2.Body.Close()

	if resp2.StatusCode == http.StatusNotFound {
		return fmt.Errorf("ingest endpoint returned 404 — server modules may not be started (check setup)")
	}
	if resp2.StatusCode != http.StatusOK {
		return fmt.Errorf("ingest endpoint returned %d", resp2.StatusCode)
	}

	c.logger.Info("Ingest endpoint OK", "url", ingestURL)
	c.setStatus(StatusConnected)
	return nil
}
