// Package mesh implements the inter-app communication network.
// Member apps register their endpoints with the mesh, discover other apps,
// and route requests through the forge server.
package mesh

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/bus"
	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
)

// Network is the mesh service that manages app discovery and routing.
type Network struct {
	mu        sync.RWMutex
	hub       *bus.Hub
	db        *sql.DB
	directory map[string]*protocol.MeshAppEntry
	handlers  map[string]RequestHandler // appID -> handler
}

// RequestHandler processes an incoming mesh request for an app.
type RequestHandler func(protocol.MeshRequest) (*protocol.MeshResponse, error)

// NewNetwork creates a mesh network backed by the bus and database.
func NewNetwork(hub *bus.Hub, db *sql.DB) *Network {
	return &Network{
		hub:       hub,
		db:        db,
		directory: make(map[string]*protocol.MeshAppEntry),
		handlers:  make(map[string]RequestHandler),
	}
}

// Register adds or updates an app in the mesh directory.
func (n *Network) Register(entry protocol.MeshAppEntry) {
	n.mu.Lock()
	defer n.mu.Unlock()

	entry.Online = true
	entry.LastSeen = time.Now()
	n.directory[entry.AppID] = &entry

	// Announce on the bus.
	n.hub.Publish(&protocol.Message{
		Type:    protocol.MsgMeshDiscover,
		From:    entry.AppID,
		To:      "all",
		Channel: protocol.ChanMesh,
		Body:    fmt.Sprintf("app %s (%s) joined the mesh", entry.Name, entry.AppID),
		Metadata: map[string]string{
			"app_id":    entry.AppID,
			"member_id": entry.MemberID,
			"name":      entry.Name,
		},
		Timestamp: time.Now(),
	})
}

// Unregister removes an app from the mesh.
func (n *Network) Unregister(appID string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if entry, ok := n.directory[appID]; ok {
		entry.Online = false
	}
}

// Directory returns the current mesh directory.
func (n *Network) Directory() *protocol.MeshDirectory {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return &protocol.MeshDirectory{
		Apps:      n.directory,
		UpdatedAt: time.Now(),
	}
}

// SetHandler registers a callback for handling mesh requests to an app.
func (n *Network) SetHandler(appID string, handler RequestHandler) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handlers[appID] = handler
}

// RouteRequest sends a mesh request to the target app's handler.
func (n *Network) RouteRequest(req protocol.MeshRequest) (*protocol.MeshResponse, error) {
	n.mu.RLock()
	handler, ok := n.handlers[req.ToApp]
	entry, exists := n.directory[req.ToApp]
	n.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("app %s not found in mesh", req.ToApp)
	}
	if !entry.Online {
		return nil, fmt.Errorf("app %s is offline", req.ToApp)
	}

	if ok {
		return handler(req)
	}

	// If no direct handler, publish as a bus message and return pending.
	n.hub.Publish(&protocol.Message{
		Type:    protocol.MsgMeshRequest,
		From:    req.FromApp,
		To:      req.ToApp,
		Channel: protocol.ChanMesh,
		Body:    req.Body,
		Metadata: map[string]string{
			"method":     req.Method,
			"path":       req.Path,
			"request_id": req.ID,
		},
		Timestamp: time.Now(),
	})

	return &protocol.MeshResponse{
		RequestID:  req.ID,
		StatusCode: 202,
		Body:       `{"status":"queued","message":"request routed via bus"}`,
		Timestamp:  time.Now(),
	}, nil
}

// FindAppsWithEndpoint searches the directory for apps exposing a given path.
func (n *Network) FindAppsWithEndpoint(path string) []*protocol.MeshAppEntry {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var matches []*protocol.MeshAppEntry
	for _, entry := range n.directory {
		if !entry.Online {
			continue
		}
		for _, ep := range entry.Endpoints {
			if ep.Path == path {
				matches = append(matches, entry)
				break
			}
		}
	}
	return matches
}
