package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"name":   "forge",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// --- Messages ---

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	var msg protocol.Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	s.hub.Publish(&msg)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "published", "id": msg.ID})
}

func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	channel := r.URL.Query().Get("channel")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	msgs := s.hub.History(channel, limit)
	json.NewEncoder(w).Encode(msgs)
}

// --- OAuth ---

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	member, err := s.auth.Bootstrap(req.Name, req.Email, req.Password)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(member)
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MemberID string `json:"member_id"`
		ValidFor string `json:"valid_for"` // e.g. "72h"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	dur, _ := time.ParseDuration(req.ValidFor)
	if dur == 0 {
		dur = 72 * time.Hour
	}
	invite, err := s.auth.CreateInvite(req.MemberID, dur)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(invite)
}

func (s *Server) handleJoinWithInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code     string `json:"code"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	member, err := s.auth.RedeemInvite(req.Code, req.Name, req.Email, req.Password)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(member)
}

func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	var req protocol.AuthorizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	memberID := r.Header.Get("X-Member-ID")
	if memberID == "" {
		memberID = r.URL.Query().Get("member_id")
	}
	code, err := s.auth.Authorize(req, memberID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"code": code})
}

func (s *Server) handleTokenExchange(w http.ResponseWriter, r *http.Request) {
	var req protocol.TokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	token, err := s.auth.Exchange(req)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(token)
}

func (s *Server) handleValidateToken(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, `{"error":"token required"}`, http.StatusBadRequest)
		return
	}
	token, err := s.auth.ValidateToken(tokenStr)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnauthorized)
		return
	}
	json.NewEncoder(w).Encode(token)
}

// --- Apps ---

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MemberID    string   `json:"member_id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		RedirectURI string   `json:"redirect_uri"`
		Scopes      []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	app, err := s.auth.CreateApp(req.MemberID, req.Name, req.Description, req.RedirectURI, req.Scopes)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(app)
}

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	memberID := r.URL.Query().Get("member_id")
	rows, err := s.db.Conn().Query(
		`SELECT id, member_id, name, description, client_id, redirect_uri, scopes, mesh_enabled, created_at
		 FROM member_apps WHERE member_id = ? OR ? = ''`,
		memberID, memberID,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var apps []map[string]interface{}
	for rows.Next() {
		var id, mid, name, desc, cid, redirect, scopes string
		var meshEnabled bool
		var createdAt time.Time
		rows.Scan(&id, &mid, &name, &desc, &cid, &redirect, &scopes, &meshEnabled, &createdAt)
		apps = append(apps, map[string]interface{}{
			"id": id, "member_id": mid, "name": name, "description": desc,
			"client_id": cid, "redirect_uri": redirect, "scopes": scopes,
			"mesh_enabled": meshEnabled, "created_at": createdAt,
		})
	}
	json.NewEncoder(w).Encode(apps)
}

// --- Mesh ---

func (s *Server) handleMeshDirectory(w http.ResponseWriter, r *http.Request) {
	dir := s.mesh.Directory()
	json.NewEncoder(w).Encode(dir)
}

func (s *Server) handleMeshRegister(w http.ResponseWriter, r *http.Request) {
	var entry protocol.MeshAppEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	s.mesh.Register(entry)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func (s *Server) handleMeshRequest(w http.ResponseWriter, r *http.Request) {
	var req protocol.MeshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	resp, err := s.mesh.RouteRequest(req)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	json.NewEncoder(w).Encode(resp)
}

// --- Enhancements ---

func (s *Server) handleCreateEnhancement(w http.ResponseWriter, r *http.Request) {
	var enh protocol.Enhancement
	if err := json.NewDecoder(r.Body).Decode(&enh); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	enh.ID = time.Now().Format("20060102150405.000000000")
	enh.CreatedAt = time.Now()
	enh.Status = protocol.EnhanceProposed

	_, err := s.db.Conn().Exec(
		`INSERT INTO enhancements (id, from_app_id, to_app_id, title, description, priority, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		enh.ID, enh.FromAppID, enh.ToAppID, enh.Title, enh.Description, enh.Priority, enh.Status,
	)
	if err != nil {
		http.Error(w, `{"error":"insert failed"}`, http.StatusInternalServerError)
		return
	}

	// Notify the target app via the bus.
	s.hub.Publish(&protocol.Message{
		Type:    protocol.MsgMeshEnhance,
		From:    enh.FromAppID,
		To:      enh.ToAppID,
		Channel: protocol.ChanMesh,
		Body:    enh.Title + ": " + enh.Description,
		Metadata: map[string]string{
			"enhancement_id": enh.ID,
			"priority":       string(enh.Priority),
		},
		Timestamp: time.Now(),
	})

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(enh)
}

func (s *Server) handleListEnhancements(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("app_id")
	rows, err := s.db.Conn().Query(
		`SELECT id, from_app_id, to_app_id, title, description, priority, status, created_at
		 FROM enhancements WHERE to_app_id = ? OR from_app_id = ? OR ? = ''
		 ORDER BY created_at DESC`,
		appID, appID, appID,
	)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var enhs []map[string]interface{}
	for rows.Next() {
		var id, from, to, title, desc, pri, status string
		var createdAt time.Time
		rows.Scan(&id, &from, &to, &title, &desc, &pri, &status, &createdAt)
		enhs = append(enhs, map[string]interface{}{
			"id": id, "from_app_id": from, "to_app_id": to,
			"title": title, "description": desc, "priority": pri,
			"status": status, "created_at": createdAt,
		})
	}
	json.NewEncoder(w).Encode(enhs)
}

func (s *Server) handleVoteEnhancement(w http.ResponseWriter, r *http.Request) {
	// Extract enhancement ID from URL path: /api/enhancements/{id}/vote
	parts := strings.Split(r.URL.Path, "/")
	enhID := ""
	for i, p := range parts {
		if p == "enhancements" && i+1 < len(parts) {
			enhID = parts[i+1]
			break
		}
	}
	var req struct {
		MemberID string `json:"member_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	s.db.Conn().Exec(
		`INSERT OR IGNORE INTO enhancement_votes (enhancement_id, member_id) VALUES (?, ?)`,
		enhID, req.MemberID,
	)
	json.NewEncoder(w).Encode(map[string]string{"status": "voted"})
}

// handleEnhancementsRouter routes sub-path requests under /api/enhancements/.
func (s *Server) handleEnhancementsRouter(w http.ResponseWriter, r *http.Request) {
	// Match pattern: /api/enhancements/{id}/vote
	if strings.HasSuffix(r.URL.Path, "/vote") && r.Method == http.MethodPost {
		s.handleVoteEnhancement(w, r)
		return
	}
	http.NotFound(w, r)
}

// --- Repos ---

func (s *Server) handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	// Just record in DB; actual bare repo creation happens via git manager.
	id := time.Now().Format("20060102150405.000000000")
	_, err := s.db.Conn().Exec(
		`INSERT INTO repos (id, name, path, description) VALUES (?, ?, ?, ?)`,
		id, req.Name, s.dataDir+"/repos/"+req.Name+".git", req.Description,
	)
	if err != nil {
		http.Error(w, `{"error":"insert failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id, "name": req.Name})
}

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Conn().Query(`SELECT id, name, description, created_at FROM repos`)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var repos []map[string]interface{}
	for rows.Next() {
		var id, name, desc string
		var createdAt time.Time
		rows.Scan(&id, &name, &desc, &createdAt)
		repos = append(repos, map[string]interface{}{
			"id": id, "name": name, "description": desc, "created_at": createdAt,
		})
	}
	json.NewEncoder(w).Encode(repos)
}

// --- Agents ---

func (s *Server) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	var agent protocol.Agent
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	agent.StartedAt = time.Now()
	// Broadcast agent join on the bus.
	s.hub.Publish(&protocol.Message{
		Type:    protocol.MsgAgentJoin,
		From:    "system",
		To:      "all",
		Channel: protocol.ChanAgents,
		Body:    agent.Name + " (" + string(agent.CLIType) + ") registered",
		Metadata: map[string]string{
			"agent_id": agent.ID,
			"cli_type": string(agent.CLIType),
			"is_alpha": strconv.FormatBool(agent.IsAlpha),
		},
		Timestamp: time.Now(),
	})
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(agent)
}

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	// Return agents from recent bus messages (lightweight; no DB for agents).
	msgs := s.hub.History(protocol.ChanAgents, 100)
	json.NewEncoder(w).Encode(msgs)
}

// --- Instructions ---

func (s *Server) handleUpdateInstructions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
		return
	}

	var update protocol.InstructionUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// Broadcast the instruction update to all agents.
	s.hub.Publish(&protocol.Message{
		Type:    protocol.MsgInstruction,
		From:    "system",
		To:      "all",
		Channel: protocol.ChanSystem,
		Body:    string(body),
		Metadata: map[string]string{
			"version":       strconv.Itoa(update.NewInstructions.Version),
			"changes":       update.ChangesSummary,
			"grace_period":  strconv.Itoa(update.GracePeriodSec),
		},
		Timestamp: time.Now(),
	})

	json.NewEncoder(w).Encode(map[string]string{
		"status":  "broadcast",
		"version": strconv.Itoa(update.NewInstructions.Version),
	})
}

func (s *Server) handleGetInstructions(w http.ResponseWriter, r *http.Request) {
	// Return the most recent instruction message from the bus.
	msgs := s.hub.History(protocol.ChanSystem, 100)
	for _, m := range msgs {
		if m.Type == protocol.MsgInstruction {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(m.Body))
			return
		}
	}
	http.Error(w, `{"error":"no instructions found"}`, http.StatusNotFound)
}
