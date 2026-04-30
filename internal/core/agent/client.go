package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// ConnStatus represents the current connection state
type ConnStatus string

const (
	StatusConnected    ConnStatus = "connected"
	StatusDisconnected ConnStatus = "disconnected"
	StatusConnecting   ConnStatus = "connecting"
)

// Client runs as an agent, collecting data and sending it to a remote server
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

func NewClient(serverURL, agentName, agentToken string, logger *slog.Logger) *Client {
	return &Client{
		serverURL:  serverURL,
		agentName:  agentName,
		agentToken: agentToken,
		logger:     logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		status:     StatusDisconnected,
	}
}

// Status returns current connection status
func (c *Client) Status() ConnStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// Stats returns agent statistics
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

// IngestPayload is sent to the server's /api/netmon/ingest endpoint
type IngestPayload struct {
	Agent string      `json:"agent"`
	Flows interface{} `json:"flows"`
}

// SendFlows posts collected flows to the server
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
		// Read response body for error details
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

// Ping checks connectivity to the server by testing the ingest endpoint
func (c *Client) Ping(ctx context.Context) error {
	// First check basic server reachability
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

	// Now test the ingest endpoint is actually registered
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
