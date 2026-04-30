package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Client runs as an agent, collecting data and sending it to a remote server
type Client struct {
	serverURL  string
	agentName  string
	agentToken string
	logger     *slog.Logger
	httpClient *http.Client
}

func NewClient(serverURL, agentName, agentToken string, logger *slog.Logger) *Client {
	return &Client{
		serverURL:  serverURL,
		agentName:  agentName,
		agentToken: agentToken,
		logger:     logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// IngestPayload is sent to the server's /api/netmon/ingest endpoint
type IngestPayload struct {
	Agent string      `json:"agent"`
	Flows interface{} `json:"flows"`
}

// SendFlows posts collected flows to the server
func (c *Client) SendFlows(ctx context.Context, flows interface{}) error {
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
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var result struct {
		Ingested int `json:"ingested"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	c.logger.Info("Sent flows to server", "ingested", result.Ingested)
	return nil
}

// Ping checks connectivity to the server
func (c *Client) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/modules", c.serverURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach server at %s: %w", c.serverURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}
