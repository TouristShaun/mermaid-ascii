// Package bridge connects the local Hive orchestrator to the remote Forge
// server. It provides HTTP API calls and WebSocket management.
package bridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
)

// Client communicates with the Forge server.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a bridge client pointing at the forge server.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HealthCheck verifies the forge server is reachable.
func (c *Client) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}

// RegisterAgent registers an agent with the forge server.
func (c *Client) RegisterAgent(agent *protocol.Agent) error {
	return c.postJSON("/api/agents/register", agent)
}

// ListAgents returns all agents known to the forge.
func (c *Client) ListAgents() (json.RawMessage, error) {
	return c.getJSON("/api/agents")
}

// SendMessage sends a chat message from the operator to an agent.
func (c *Client) SendMessage(agentID, body string) error {
	msg := protocol.Message{
		ID:        time.Now().Format("20060102150405.000"),
		Type:      protocol.MsgChat,
		From:      "operator",
		To:        agentID,
		Channel:   protocol.ChanAgents,
		Body:      body,
		Timestamp: time.Now(),
	}
	return c.postJSON("/api/messages", msg)
}

// UpdateInstructions broadcasts new instructions to all agents.
func (c *Client) UpdateInstructions(instrSet *protocol.InstructionSet, graceSec int) error {
	update := protocol.InstructionUpdate{
		NewInstructions: *instrSet,
		ChangesSummary:  fmt.Sprintf("Updated to version %d", instrSet.Version),
		GracePeriodSec:  graceSec,
	}
	return c.postJSON("/api/instructions", update)
}

// BroadcastStop sends a shutdown signal to all agents.
func (c *Client) BroadcastStop() error {
	msg := protocol.Message{
		ID:        time.Now().Format("20060102150405.000"),
		Type:      protocol.MsgChat,
		From:      "system",
		To:        "all",
		Channel:   protocol.ChanSystem,
		Body:      "SHUTDOWN: Please finish your current task and stop.",
		Timestamp: time.Now(),
	}
	return c.postJSON("/api/messages", msg)
}

// GetMessages retrieves recent messages from the bus.
func (c *Client) GetMessages(channel string, limit int) ([]*protocol.Message, error) {
	url := fmt.Sprintf("/api/messages?channel=%s&limit=%d", channel, limit)
	data, err := c.getJSON(url)
	if err != nil {
		return nil, err
	}
	var msgs []*protocol.Message
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}
	return msgs, nil
}

func (c *Client) postJSON(path string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("post %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return nil
}

func (c *Client) getJSON(path string) (json.RawMessage, error) {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get %s returned %d: %s", path, resp.StatusCode, string(data))
	}
	return data, nil
}
