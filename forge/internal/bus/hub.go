// Package bus implements the WebSocket-based message bus that agents use to
// communicate with each other and with the forge system. The Hub manages
// connections, channels, and message routing.
package bus

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
	"github.com/gorilla/websocket"
)

// Client is a connected agent on the bus.
type Client struct {
	AgentID  string
	Conn     *websocket.Conn
	Channels map[string]bool
	Send     chan []byte
	hub      *Hub
}

// Hub maintains active clients, channels, and routes messages.
type Hub struct {
	mu          sync.RWMutex
	clients     map[string]*Client
	channels    map[string]map[string]*Client // channel -> agentID -> client
	register    chan *Client
	unregister  chan *Client
	broadcast   chan *protocol.Message
	history     []*protocol.Message // recent message buffer
	maxHistory  int
	done        chan struct{}
}

// NewHub creates a new message bus hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		channels:   make(map[string]map[string]*Client),
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
		broadcast:  make(chan *protocol.Message, 256),
		maxHistory: 1000,
		done:       make(chan struct{}),
	}
}

// Run starts the hub's main event loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.AgentID] = client
			// Auto-subscribe to system and agents channels.
			h.subscribe(client, protocol.ChanSystem)
			h.subscribe(client, protocol.ChanAgents)
			h.mu.Unlock()

			log.Printf("[bus] agent %s connected", client.AgentID)
			h.Publish(&protocol.Message{
				ID:        generateID(),
				Type:      protocol.MsgAgentJoin,
				From:      "system",
				To:        "all",
				Channel:   protocol.ChanSystem,
				Body:      client.AgentID,
				Timestamp: time.Now(),
			})

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.AgentID]; ok {
				delete(h.clients, client.AgentID)
				for ch, members := range h.channels {
					delete(members, client.AgentID)
					if len(members) == 0 {
						delete(h.channels, ch)
					}
				}
				close(client.Send)
			}
			h.mu.Unlock()

			log.Printf("[bus] agent %s disconnected", client.AgentID)
			h.Publish(&protocol.Message{
				ID:        generateID(),
				Type:      protocol.MsgAgentLeave,
				From:      "system",
				To:        "all",
				Channel:   protocol.ChanSystem,
				Body:      client.AgentID,
				Timestamp: time.Now(),
			})

		case msg := <-h.broadcast:
			h.mu.RLock()
			h.history = append(h.history, msg)
			if len(h.history) > h.maxHistory {
				h.history = h.history[1:]
			}

			// Route message based on channel and recipient.
			targets := h.resolveTargets(msg)
			data, _ := json.Marshal(msg)
			for _, t := range targets {
				select {
				case t.Send <- data:
				default:
					log.Printf("[bus] dropping message to slow client %s", t.AgentID)
				}
			}
			h.mu.RUnlock()

		case <-h.done:
			h.mu.Lock()
			for _, c := range h.clients {
				close(c.Send)
			}
			h.mu.Unlock()
			return
		}
	}
}

// Publish sends a message through the bus.
func (h *Hub) Publish(msg *protocol.Message) {
	select {
	case h.broadcast <- msg:
	default:
		log.Printf("[bus] broadcast channel full, dropping message %s", msg.ID)
	}
}

// Subscribe adds a client to a channel.
func (h *Hub) Subscribe(agentID, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c, ok := h.clients[agentID]; ok {
		h.subscribe(c, channel)
	}
}

func (h *Hub) subscribe(client *Client, channel string) {
	if _, ok := h.channels[channel]; !ok {
		h.channels[channel] = make(map[string]*Client)
	}
	h.channels[channel][client.AgentID] = client
	client.Channels[channel] = true
}

// Register adds a client to the hub.
func (h *Hub) Register(c *Client) {
	h.register <- c
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}

// Shutdown stops the hub.
func (h *Hub) Shutdown() {
	close(h.done)
}

// History returns recent messages, optionally filtered by channel.
func (h *Hub) History(channel string, limit int) []*protocol.Message {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var msgs []*protocol.Message
	for i := len(h.history) - 1; i >= 0 && len(msgs) < limit; i-- {
		if channel == "" || h.history[i].Channel == channel {
			msgs = append(msgs, h.history[i])
		}
	}
	return msgs
}

// resolveTargets determines which clients should receive a message.
func (h *Hub) resolveTargets(msg *protocol.Message) []*Client {
	var targets []*Client

	// Direct message to a specific agent.
	if msg.To != "" && msg.To != "all" && msg.To != "alphas" {
		if c, ok := h.clients[msg.To]; ok {
			return []*Client{c}
		}
		return nil
	}

	// Channel-based routing.
	if members, ok := h.channels[msg.Channel]; ok {
		for id, c := range members {
			if id != msg.From { // don't echo back to sender
				targets = append(targets, c)
			}
		}
	}
	return targets
}

// ClientCount returns the number of currently connected WebSocket clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// generateID produces a simple unique ID.
func generateID() string {
	return time.Now().Format("20060102150405.000000000")
}
