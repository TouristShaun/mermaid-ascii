// Package api provides the HTTP API for the Forge server. It wires together
// the git layer, message bus, OAuth, and mesh into a single HTTP router.
package api

import (
	"net/http"

	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/auth"
	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/bus"
	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/mesh"
	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/store"
)

// Server holds all dependencies for the HTTP API.
type Server struct {
	hub     *bus.Hub
	db      *store.DB
	auth    *auth.Service
	mesh    *mesh.Network
	mw      *auth.TokenMiddleware
	dataDir string
}

// NewServer creates a new API server with all subsystems initialized.
func NewServer(hub *bus.Hub, db *store.DB, dataDir string) *Server {
	authSvc := auth.NewService(db.Conn())
	meshNet := mesh.NewNetwork(hub, db.Conn())

	return &Server{
		hub:     hub,
		db:      db,
		auth:    authSvc,
		mesh:    meshNet,
		mw:      auth.NewTokenMiddleware(authSvc),
		dataDir: dataDir,
	}
}

// Router returns the fully-configured HTTP mux.
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	// --- Health ---
	mux.HandleFunc("/health", s.handleHealth)

	// --- WebSocket bus ---
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		bus.HandleWebSocket(s.hub, w, r)
	})

	// --- Bus REST API (for agents that can't do WebSocket) ---
	mux.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			s.handlePostMessage(w, r)
		default:
			s.handleGetMessages(w, r)
		}
	})

	// --- OAuth endpoints ---
	mux.HandleFunc("/api/auth/bootstrap", s.handleBootstrap)
	mux.HandleFunc("/api/auth/invite", s.handleCreateInvite)
	mux.HandleFunc("/api/auth/join", s.handleJoinWithInvite)
	mux.HandleFunc("/api/auth/authorize", s.handleAuthorize)
	mux.HandleFunc("/api/auth/token", s.handleTokenExchange)
	mux.HandleFunc("/api/auth/validate", s.handleValidateToken)

	// --- App management ---
	mux.HandleFunc("/api/apps", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			s.handleCreateApp(w, r)
		default:
			s.handleListApps(w, r)
		}
	})

	// --- Mesh ---
	mux.HandleFunc("/api/mesh/directory", s.handleMeshDirectory)
	mux.HandleFunc("/api/mesh/register", s.handleMeshRegister)
	mux.HandleFunc("/api/mesh/request", s.handleMeshRequest)

	// --- Enhancements ---
	// The /api/enhancements/ prefix with trailing slash catches sub-paths
	// like /api/enhancements/{id}/vote via path parsing in the handler.
	mux.HandleFunc("/api/enhancements/", s.handleEnhancementsRouter)
	mux.HandleFunc("/api/enhancements", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			s.handleCreateEnhancement(w, r)
		default:
			s.handleListEnhancements(w, r)
		}
	})

	// --- Git repos ---
	mux.HandleFunc("/api/repos", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			s.handleCreateRepo(w, r)
		default:
			s.handleListRepos(w, r)
		}
	})

	// --- Agent registry ---
	mux.HandleFunc("/api/agents/register", s.handleRegisterAgent)
	mux.HandleFunc("/api/agents", s.handleListAgents)

	// --- Instructions ---
	mux.HandleFunc("/api/instructions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			s.handleUpdateInstructions(w, r)
		default:
			s.handleGetInstructions(w, r)
		}
	})

	// --- Admin cockpit ---
	mux.HandleFunc("/cockpit", s.handleCockpit)
	mux.HandleFunc("/api/admin/stats", s.handleAdminStats)
	mux.HandleFunc("/api/admin/members", s.handleAdminMembers)

	// CORS middleware for development.
	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
