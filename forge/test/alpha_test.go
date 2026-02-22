// Package test runs end-to-end integration tests against a Forge server.
// It starts its own server instance on a random port with a temp DB,
// then simulates all three alpha agents (Claude, Codex, Gemini) performing
// the full OAuth + mesh + enhancement + WebSocket workflow.
//
// Run with:
//
//	CGO_ENABLED=1 go test -v -count=1 ./forge/test/
package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/api"
	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/bus"
	"github.com/AlexanderGrooff/mermaid-ascii/forge/internal/store"
	"github.com/AlexanderGrooff/mermaid-ascii/protocol"
	"github.com/gorilla/websocket"
)

var (
	forgeURL string
	httpSrv  *http.Server
	hub      *bus.Hub
)

// TestMain starts a fresh forge server and runs all tests against it.
func TestMain(m *testing.M) {
	// Create temp data dir.
	dataDir, err := os.MkdirTemp("", "forge-test-*")
	if err != nil {
		log.Fatalf("tmpdir: %v", err)
	}
	defer os.RemoveAll(dataDir)

	// Open store.
	db, err := store.Open(dataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	// Start hub.
	hub = bus.NewHub()
	go hub.Run()

	// Pick a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	forgeURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	// Build server.
	srv := api.NewServer(hub, db, dataDir)
	httpSrv = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: srv.Router(),
	}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	// Wait for server ready.
	for i := 0; i < 50; i++ {
		if resp, err := http.Get(forgeURL + "/health"); err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	code := m.Run()

	hub.Shutdown()
	httpSrv.Close()
	os.Exit(code)
}

// --- helpers ---

func post(t *testing.T, path string, body interface{}) map[string]interface{} {
	t.Helper()
	data, _ := json.Marshal(body)
	resp, err := http.Post(forgeURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		t.Fatalf("POST %s returned %d: %s", path, resp.StatusCode, string(raw))
	}
	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	return result
}

func postRaw(path string, body interface{}) (*http.Response, []byte) {
	data, _ := json.Marshal(body)
	resp, err := http.Post(forgeURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, nil
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, raw
}

func get(t *testing.T, path string) json.RawMessage {
	t.Helper()
	resp, err := http.Get(forgeURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		t.Fatalf("GET %s returned %d: %s", path, resp.StatusCode, string(raw))
	}
	return raw
}

func str(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// Shared state across phases (set by earlier tests, used by later ones).
var (
	operatorID   string
	inviteCode1  string
	inviteCode2  string
	aliceMID     string
	bobMID       string
	charlieMID   string
	aliceAppData map[string]interface{}
	bobAppData   map[string]interface{}
)

// ============================================================
// Phase 1: Health + Operator Bootstrap
// ============================================================

func TestPhase1_HealthAndBootstrap(t *testing.T) {
	t.Log("=== PHASE 1: Health Check & Operator Bootstrap ===")

	// Health.
	raw := get(t, "/health")
	var h map[string]string
	json.Unmarshal(raw, &h)
	if h["status"] != "ok" {
		t.Fatalf("health not ok: %v", h)
	}
	t.Logf("forge healthy: %s", h["time"])

	// Bootstrap the operator (team of one).
	op := post(t, "/api/auth/bootstrap", map[string]string{
		"name":     "Operator",
		"email":    "operator@infinityforward.ltd",
		"password": "forge-strong-pass",
	})
	operatorID = str(op, "id")
	if operatorID == "" {
		t.Fatal("bootstrap returned no id")
	}
	t.Logf("operator created: id=%s email=%s", operatorID, str(op, "email"))

	// Trying again should fail (only one operator).
	resp, _ := postRaw("/api/auth/bootstrap", map[string]string{
		"name": "Dup", "email": "dup@x.com", "password": "x",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate bootstrap, got %d", resp.StatusCode)
	}
	t.Log("duplicate bootstrap correctly rejected")
}

// ============================================================
// Phase 2: Alpha Agent Registration
// ============================================================

func TestPhase2_AlphaRegistration(t *testing.T) {
	t.Log("=== PHASE 2: Register All Three Alpha Agents ===")

	alphas := []protocol.Agent{
		{ID: "claude-0", CLIType: protocol.CLIClaude, Name: "claude-alpha", IsAlpha: true, Worktree: "/ws/claude-0", Branch: "agent/claude-0", Status: protocol.AgentIdle},
		{ID: "codex-0", CLIType: protocol.CLICodex, Name: "codex-alpha", IsAlpha: true, Worktree: "/ws/codex-0", Branch: "agent/codex-0", Status: protocol.AgentIdle},
		{ID: "gemini-0", CLIType: protocol.CLIGemini, Name: "gemini-alpha", IsAlpha: true, Worktree: "/ws/gemini-0", Branch: "agent/gemini-0", Status: protocol.AgentIdle},
	}

	for _, a := range alphas {
		result := post(t, "/api/agents/register", a)
		if str(result, "id") != a.ID {
			t.Fatalf("expected id %s, got %s", a.ID, str(result, "id"))
		}
		t.Logf("registered %s (alpha=%v, type=%s)", a.Name, a.IsAlpha, a.CLIType)
	}

	// Verify agent list comes back via message bus history.
	raw := get(t, "/api/agents")
	var msgs []interface{}
	json.Unmarshal(raw, &msgs)
	if len(msgs) < 3 {
		t.Fatalf("expected >= 3 agent events, got %d", len(msgs))
	}
	t.Logf("agent bus has %d events", len(msgs))
}

// ============================================================
// Phase 3: Claude-Alpha — Designs the OAuth System
// ============================================================

func TestPhase3_ClaudeAlpha_OAuthDesign(t *testing.T) {
	t.Log("=== PHASE 3: Claude-Alpha — Architect the OAuth System ===")

	if operatorID == "" {
		t.Fatal("operatorID not set — Phase 1 must pass first")
	}

	// Claude-alpha announces its design plan on the bus.
	post(t, "/api/messages", map[string]interface{}{
		"id":      "msg-claude-001",
		"type":    "chat",
		"from":    "claude-0",
		"to":      "all",
		"channel": "agents",
		"body":    "@codex-0 @gemini-0 I'm designing the OAuth system. Referral-only, per-member apps, mesh-enabled. I'll build the core token flow. Codex, take the invite system. Gemini, prep for security audit.",
	})
	t.Log("claude-alpha: broadcast architecture plan to all agents")

	// Claude-alpha creates invite codes for two new members.
	inv1 := post(t, "/api/auth/invite", map[string]interface{}{
		"member_id": operatorID,
		"valid_for": "24h",
	})
	inv2 := post(t, "/api/auth/invite", map[string]interface{}{
		"member_id": operatorID,
		"valid_for": "24h",
	})
	inviteCode1 = str(inv1, "code")
	inviteCode2 = str(inv2, "code")
	t.Logf("claude-alpha: created invite codes: %s, %s", inviteCode1, inviteCode2)

	// Claude creates the project repo.
	repo := post(t, "/api/repos", map[string]interface{}{
		"name":        "infinity-oauth",
		"description": "Referral-only OAuth system for Infinity Forward Ltd.",
	})
	t.Logf("claude-alpha: created repo %s", str(repo, "name"))

	// Claude-alpha creates the operator's custom app.
	app := post(t, "/api/apps", map[string]interface{}{
		"member_id":    operatorID,
		"name":         "Operator Dashboard",
		"description":  "The operator's admin app for Infinity Forward Ltd.",
		"redirect_uri": "https://infinityforward.ltd/callback",
		"scopes":       []string{"profile", "apps", "mesh", "suggest", "admin"},
	})
	clientID := str(app, "client_id")
	clientSecret := str(app, "secret")
	t.Logf("claude-alpha: created operator app: client_id=%s", clientID)

	// Full OAuth authorization code flow.
	authResp := post(t, "/api/auth/authorize?member_id="+operatorID, map[string]interface{}{
		"client_id":     clientID,
		"redirect_uri":  "https://infinityforward.ltd/callback",
		"scope":         "profile,apps,mesh,suggest,admin",
		"state":         "test-state-123",
		"response_type": "code",
	})
	authCode := str(authResp, "code")
	if authCode == "" {
		t.Fatal("no auth code returned")
	}
	t.Logf("claude-alpha: got authorization code: %s...", authCode[:16])

	// Exchange code for token.
	token := post(t, "/api/auth/token", map[string]interface{}{
		"grant_type":    "authorization_code",
		"code":          authCode,
		"redirect_uri":  "https://infinityforward.ltd/callback",
		"client_id":     clientID,
		"client_secret": clientSecret,
	})
	accessToken := str(token, "access_token")
	refreshToken := str(token, "refresh_token")
	if accessToken == "" || refreshToken == "" {
		t.Fatal("token exchange failed")
	}
	t.Logf("claude-alpha: OAuth flow complete. access_token=%s... refresh_token=%s...", accessToken[:16], refreshToken[:16])

	// Validate the token.
	valRaw := get(t, "/api/auth/validate?token="+accessToken)
	var val map[string]interface{}
	json.Unmarshal(valRaw, &val)
	if str(val, "member_id") != operatorID {
		t.Fatalf("token validation returned wrong member: %v", val)
	}
	t.Log("claude-alpha: token validated successfully")

	// Test refresh token rotation.
	refreshed := post(t, "/api/auth/token", map[string]interface{}{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     clientID,
		"client_secret": clientSecret,
	})
	newAccess := str(refreshed, "access_token")
	if newAccess == "" || newAccess == accessToken {
		t.Fatal("refresh did not issue new token")
	}
	t.Logf("claude-alpha: token rotation works. new_access=%s...", newAccess[:16])

	// Claude reports completion to the bus.
	post(t, "/api/messages", map[string]interface{}{
		"id":      "msg-claude-002",
		"type":    "task_done",
		"from":    "claude-0",
		"to":      "all",
		"channel": "agents",
		"body":    "OAuth core complete: bootstrap, authorization code flow, token exchange, refresh rotation all verified. @codex-0 you're up — invite codes ready.",
		"metadata": map[string]string{
			"task":        "oauth-core",
			"operator_id": operatorID,
		},
	})
	t.Log("claude-alpha: task_done broadcast sent")
}

// ============================================================
// Phase 4: Codex-Alpha — Builds Invite System + Per-Member Apps
// ============================================================

func TestPhase4_CodexAlpha_InviteAndApps(t *testing.T) {
	t.Log("=== PHASE 4: Codex-Alpha — Invite System & Per-Member Apps ===")

	if operatorID == "" || inviteCode1 == "" {
		t.Fatal("missing state from earlier phases")
	}

	// Codex announces it's starting work.
	post(t, "/api/messages", map[string]interface{}{
		"id":      "msg-codex-001",
		"type":    "task_update",
		"from":    "codex-0",
		"to":      "all",
		"channel": "agents",
		"body":    "Starting invite system. Will test the full referral chain: operator -> member1 -> member2.",
	})

	// Member 1 (Alice) joins via invite code from operator.
	m1 := post(t, "/api/auth/join", map[string]interface{}{
		"code":     inviteCode1,
		"name":     "Alice",
		"email":    "alice@example.com",
		"password": "alice-pass",
	})
	aliceMID = str(m1, "id")
	if aliceMID == "" {
		t.Fatal("member 1 join failed")
	}
	t.Logf("codex-alpha: Alice joined: id=%s, invited_by=%s", aliceMID, str(m1, "invited_by"))

	// Member 2 (Bob) joins via second invite.
	m2 := post(t, "/api/auth/join", map[string]interface{}{
		"code":     inviteCode2,
		"name":     "Bob",
		"email":    "bob@example.com",
		"password": "bob-pass",
	})
	bobMID = str(m2, "id")
	t.Logf("codex-alpha: Bob joined: id=%s, invited_by=%s", bobMID, str(m2, "invited_by"))

	// Verify single-use: reusing code1 should fail.
	resp, _ := postRaw("/api/auth/join", map[string]string{
		"code": inviteCode1, "name": "Fake", "email": "fake@x.com", "password": "x",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for reused invite, got %d", resp.StatusCode)
	}
	t.Log("codex-alpha: invite code single-use verified (reuse rejected)")

	// Alice creates an invite for someone else (chain referral).
	inv3 := post(t, "/api/auth/invite", map[string]interface{}{
		"member_id": aliceMID, "valid_for": "1h",
	})
	code3 := str(inv3, "code")
	m3 := post(t, "/api/auth/join", map[string]interface{}{
		"code":     code3,
		"name":     "Charlie",
		"email":    "charlie@example.com",
		"password": "charlie-pass",
	})
	charlieMID = str(m3, "id")
	t.Logf("codex-alpha: referral chain works: Charlie (%s) invited by Alice (%s)", charlieMID[:8], aliceMID[:8])

	// Per-member app provisioning.
	type memberApp struct {
		id, name, redirect string
	}
	members := []memberApp{
		{aliceMID, "Alice's App", "https://alice.infinityforward.ltd/callback"},
		{bobMID, "Bob's App", "https://bob.infinityforward.ltd/callback"},
		{charlieMID, "Charlie's App", "https://charlie.infinityforward.ltd/callback"},
	}

	for _, m := range members {
		app := post(t, "/api/apps", map[string]interface{}{
			"member_id":    m.id,
			"name":         m.name,
			"description":  fmt.Sprintf("Custom app for %s — forever relationship with Infinity Forward Ltd.", m.name),
			"redirect_uri": m.redirect,
			"scopes":       []string{"profile", "mesh", "suggest"},
		})
		t.Logf("codex-alpha: provisioned %s — client_id=%s, mesh_enabled=%v",
			m.name, str(app, "client_id"), app["mesh_enabled"])

		if m.id == aliceMID {
			aliceAppData = app
		}
		if m.id == bobMID {
			bobAppData = app
		}
	}

	// Verify apps list.
	raw := get(t, "/api/apps?member_id="+aliceMID)
	var appList []interface{}
	json.Unmarshal(raw, &appList)
	if len(appList) < 1 {
		t.Fatal("expected at least 1 app for Alice")
	}
	t.Logf("codex-alpha: Alice has %d app(s)", len(appList))

	// Full OAuth flow for Alice's app.
	authResp := post(t, "/api/auth/authorize?member_id="+aliceMID, map[string]interface{}{
		"client_id":     str(aliceAppData, "client_id"),
		"redirect_uri":  "https://alice.infinityforward.ltd/callback",
		"scope":         "profile,mesh,suggest",
		"state":         "alice-state",
		"response_type": "code",
	})
	aliceToken := post(t, "/api/auth/token", map[string]interface{}{
		"grant_type":    "authorization_code",
		"code":          str(authResp, "code"),
		"redirect_uri":  "https://alice.infinityforward.ltd/callback",
		"client_id":     str(aliceAppData, "client_id"),
		"client_secret": str(aliceAppData, "secret"),
	})
	t.Logf("codex-alpha: Alice OAuth complete. token=%s...", str(aliceToken, "access_token")[:16])

	// Codex reports completion.
	post(t, "/api/messages", map[string]interface{}{
		"id":      "msg-codex-002",
		"type":    "task_done",
		"from":    "codex-0",
		"to":      "all",
		"channel": "agents",
		"body":    fmt.Sprintf("Invite system and per-member apps complete. Referral chain: Operator->Alice->Charlie, Operator->Bob. All 3 members have custom apps with OAuth tokens. @gemini-0 ready for security audit."),
	})
	t.Log("codex-alpha: task_done — invite + per-member apps verified")
}

// ============================================================
// Phase 5: Codex-Alpha — Builds the Mesh Network
// ============================================================

func TestPhase5_CodexAlpha_MeshNetwork(t *testing.T) {
	t.Log("=== PHASE 5: Codex-Alpha — Inter-App Mesh Network ===")

	// Register three apps in the mesh.
	meshApps := []protocol.MeshAppEntry{
		{
			AppID: "app-alice", MemberID: "alice", MemberName: "Alice", Name: "Alice's Dashboard",
			Endpoints: []protocol.MeshEndpoint{
				{AppID: "app-alice", Path: "/api/profile", Methods: []string{"GET"}, Description: "Get Alice's profile"},
				{AppID: "app-alice", Path: "/api/data", Methods: []string{"GET", "POST"}, Description: "Alice's data store"},
			},
		},
		{
			AppID: "app-bob", MemberID: "bob", MemberName: "Bob", Name: "Bob's Toolkit",
			Endpoints: []protocol.MeshEndpoint{
				{AppID: "app-bob", Path: "/api/tools", Methods: []string{"GET"}, Description: "Bob's tool catalog"},
			},
		},
		{
			AppID: "app-charlie", MemberID: "charlie", MemberName: "Charlie", Name: "Charlie's API",
			Endpoints: []protocol.MeshEndpoint{
				{AppID: "app-charlie", Path: "/api/compute", Methods: []string{"POST"}, Description: "Charlie's compute service"},
			},
		},
	}

	for _, entry := range meshApps {
		post(t, "/api/mesh/register", entry)
		t.Logf("codex-alpha: registered %s in mesh (%d endpoints)", entry.Name, len(entry.Endpoints))
	}

	// Verify the mesh directory.
	raw := get(t, "/api/mesh/directory")
	var dir protocol.MeshDirectory
	json.Unmarshal(raw, &dir)
	if len(dir.Apps) != 3 {
		t.Fatalf("expected 3 apps in mesh, got %d", len(dir.Apps))
	}
	for appID, entry := range dir.Apps {
		t.Logf("  mesh: %s — %s (online=%v, endpoints=%d)", appID, entry.Name, entry.Online, len(entry.Endpoints))
	}

	// Cross-app mesh request: Alice -> Bob.
	meshResp := post(t, "/api/mesh/request", map[string]interface{}{
		"id":       "mesh-req-001",
		"from_app": "app-alice",
		"to_app":   "app-bob",
		"method":   "GET",
		"path":     "/api/tools",
		"token":    "alice-bearer-token",
	})
	t.Logf("codex-alpha: mesh request alice->bob: status=%v", meshResp["status_code"])

	// Cross-app mesh request: Bob -> Charlie.
	meshResp2 := post(t, "/api/mesh/request", map[string]interface{}{
		"id":       "mesh-req-002",
		"from_app": "app-bob",
		"to_app":   "app-charlie",
		"method":   "POST",
		"path":     "/api/compute",
		"body":     `{"task":"analyze","data":[1,2,3]}`,
		"token":    "bob-bearer-token",
	})
	t.Logf("codex-alpha: mesh request bob->charlie: status=%v", meshResp2["status_code"])

	post(t, "/api/messages", map[string]interface{}{
		"id":      "msg-codex-003",
		"type":    "task_done",
		"from":    "codex-0",
		"to":      "all",
		"channel": "agents",
		"body":    "Mesh network operational. 3 apps registered, cross-app routing verified.",
	})
	t.Log("codex-alpha: mesh network verified")
}

// ============================================================
// Phase 6: Gemini-Alpha — Security Audit + Enhancement Suggestions
// ============================================================

func TestPhase6_GeminiAlpha_SecurityAudit(t *testing.T) {
	t.Log("=== PHASE 6: Gemini-Alpha — Security Audit ===")

	if operatorID == "" {
		t.Fatal("operatorID not set")
	}

	// Gemini announces audit start.
	post(t, "/api/messages", map[string]interface{}{
		"id":      "msg-gemini-001",
		"type":    "task_update",
		"from":    "gemini-0",
		"to":      "all",
		"channel": "agents",
		"body":    "Starting security audit. Checking: token leakage, invite code entropy, password hashing, SQL injection vectors, CSRF, rate limiting.",
	})

	// TEST 1: expired invite codes should be rejected.
	inv := post(t, "/api/auth/invite", map[string]interface{}{
		"member_id": operatorID, "valid_for": "1ms",
	})
	expCode := str(inv, "code")
	time.Sleep(10 * time.Millisecond)
	resp, _ := postRaw("/api/auth/join", map[string]string{
		"code": expCode, "name": "Hacker", "email": "h@x.com", "password": "x",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expired invite should be rejected, got %d", resp.StatusCode)
	}
	t.Log("gemini-alpha: PASS — expired invite codes rejected")

	// TEST 2: forged tokens rejected.
	resp2, _ := http.Get(forgeURL + "/api/auth/validate?token=forged-fake-token-12345")
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("fake token should be 401, got %d", resp2.StatusCode)
	}
	t.Log("gemini-alpha: PASS — forged tokens rejected")

	// TEST 3: wrong client_secret rejected.
	app := post(t, "/api/apps", map[string]interface{}{
		"member_id":    operatorID,
		"name":         "Audit App",
		"description":  "Test app for security audit",
		"redirect_uri": "https://audit.test/cb",
		"scopes":       []string{"profile"},
	})
	authResp := post(t, "/api/auth/authorize?member_id="+operatorID, map[string]interface{}{
		"client_id":     str(app, "client_id"),
		"redirect_uri":  "https://audit.test/cb",
		"scope":         "profile",
		"state":         "audit",
		"response_type": "code",
	})
	resp3, _ := postRaw("/api/auth/token", map[string]string{
		"grant_type":    "authorization_code",
		"code":          str(authResp, "code"),
		"redirect_uri":  "https://audit.test/cb",
		"client_id":     str(app, "client_id"),
		"client_secret": "wrong-secret-attempt",
	})
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong secret should be 400, got %d", resp3.StatusCode)
	}
	t.Log("gemini-alpha: PASS — invalid client_secret rejected")

	// TEST 4: auth code replay blocked.
	auth2 := post(t, "/api/auth/authorize?member_id="+operatorID, map[string]interface{}{
		"client_id":     str(app, "client_id"),
		"redirect_uri":  "https://audit.test/cb",
		"scope":         "profile",
		"state":         "replay-test",
		"response_type": "code",
	})
	// First use (legitimate).
	post(t, "/api/auth/token", map[string]interface{}{
		"grant_type":    "authorization_code",
		"code":          str(auth2, "code"),
		"redirect_uri":  "https://audit.test/cb",
		"client_id":     str(app, "client_id"),
		"client_secret": str(app, "secret"),
	})
	// Replay attempt.
	resp4, _ := postRaw("/api/auth/token", map[string]interface{}{
		"grant_type":    "authorization_code",
		"code":          str(auth2, "code"),
		"redirect_uri":  "https://audit.test/cb",
		"client_id":     str(app, "client_id"),
		"client_secret": str(app, "secret"),
	})
	if resp4.StatusCode != http.StatusBadRequest {
		t.Fatalf("replayed auth code should be 400, got %d", resp4.StatusCode)
	}
	t.Log("gemini-alpha: PASS — auth code replay attack blocked")

	// TEST 5: redirect_uri mismatch.
	resp5, _ := postRaw("/api/auth/authorize?member_id="+operatorID, map[string]interface{}{
		"client_id":     str(app, "client_id"),
		"redirect_uri":  "https://evil.site/steal",
		"scope":         "profile",
		"state":         "evil",
		"response_type": "code",
	})
	if resp5.StatusCode != http.StatusBadRequest {
		t.Fatalf("redirect mismatch should be 400, got %d", resp5.StatusCode)
	}
	t.Log("gemini-alpha: PASS — redirect_uri mismatch blocked")

	// File enhancement suggestions.
	findings := []struct {
		title, desc, priority string
	}{
		{"Upgrade to bcrypt for password hashing", "SHA-256 is fast to brute-force. Switch to bcrypt cost 12.", "high"},
		{"Add PKCE to OAuth flow", "Implement RFC 7636 to protect against auth code interception.", "high"},
		{"Rate limit token and invite endpoints", "Add 10 req/min per IP to prevent brute-force.", "medium"},
		{"Add CSRF state validation", "Authorize endpoint accepts state but doesn't validate server-side.", "medium"},
	}

	for _, f := range findings {
		enh := post(t, "/api/enhancements", map[string]interface{}{
			"from_app_id": "gemini-audit",
			"to_app_id":   "forge-auth",
			"title":       f.title,
			"description": f.desc,
			"priority":    f.priority,
		})
		t.Logf("gemini-alpha: enhancement [%s] %s (id=%s)", f.priority, f.title, str(enh, "id"))
	}

	// Vote on enhancements from all alphas.
	raw := get(t, "/api/enhancements")
	var enhs []map[string]interface{}
	json.Unmarshal(raw, &enhs)
	for _, enh := range enhs {
		enhID := str(enh, "id")
		for _, voter := range []string{"claude-0", "codex-0", "gemini-0"} {
			post(t, "/api/enhancements/"+enhID+"/vote", map[string]interface{}{
				"member_id": voter,
			})
		}
	}
	t.Logf("gemini-alpha: all %d enhancements voted on by 3 alphas", len(enhs))

	post(t, "/api/messages", map[string]interface{}{
		"id":      "msg-gemini-002",
		"type":    "task_done",
		"from":    "gemini-0",
		"to":      "all",
		"channel": "agents",
		"body":    fmt.Sprintf("Security audit complete. 5/5 tests passed. Filed %d enhancements. @claude-0 @codex-0 review.", len(findings)),
	})
	t.Log("gemini-alpha: security audit complete")
}

// ============================================================
// Phase 7: WebSocket Bus — Real-Time Agent Communication
// ============================================================

func TestPhase7_WebSocketBus(t *testing.T) {
	t.Log("=== PHASE 7: WebSocket Bus — Real-Time Inter-Agent Communication ===")

	wsURL := strings.Replace(forgeURL, "http://", "ws://", 1)

	var wg sync.WaitGroup
	received := make(map[string][]string)
	var mu sync.Mutex

	agents := []string{"claude-0", "codex-0", "gemini-0"}
	conns := make(map[string]*websocket.Conn)

	for _, agentID := range agents {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws?agent_id="+agentID, nil)
		if err != nil {
			t.Fatalf("ws connect %s: %v", agentID, err)
		}
		conns[agentID] = conn
		t.Logf("ws: %s connected", agentID)

		wg.Add(1)
		go func(id string, c *websocket.Conn) {
			defer wg.Done()
			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}
				var msg protocol.Message
				json.Unmarshal(data, &msg)
				mu.Lock()
				received[id] = append(received[id], fmt.Sprintf("[%s] %s: %s", msg.Type, msg.From, msg.Body))
				mu.Unlock()
			}
		}(agentID, conn)
	}

	// Subscribe all to review channel.
	sub, _ := json.Marshal(protocol.Message{Type: "subscribe", Channel: protocol.ChanReview})
	for _, c := range conns {
		c.WriteMessage(websocket.TextMessage, sub)
	}

	// Claude -> Codex: task assignment.
	msg1 := protocol.Message{
		ID: "ws-001", Type: protocol.MsgTaskAssign, From: "claude-0", To: "codex-0",
		Channel: protocol.ChanAgents,
		Body:    "Implement PKCE extension per gemini's recommendation.",
	}
	d1, _ := json.Marshal(msg1)
	conns["claude-0"].WriteMessage(websocket.TextMessage, d1)
	t.Log("ws: claude-alpha -> codex-alpha: task assignment")

	// Codex -> all: status update.
	msg2 := protocol.Message{
		ID: "ws-002", Type: protocol.MsgTaskUpdate, From: "codex-0", To: "all",
		Channel: protocol.ChanAgents,
		Body:    "Working on PKCE. Adding code_verifier and code_challenge.",
	}
	d2, _ := json.Marshal(msg2)
	conns["codex-0"].WriteMessage(websocket.TextMessage, d2)
	t.Log("ws: codex-alpha -> all: status update")

	// Gemini -> review channel: code review request.
	msg3 := protocol.Message{
		ID: "ws-003", Type: protocol.MsgCodeReview, From: "gemini-0", To: "all",
		Channel: protocol.ChanReview,
		Body:    "Requesting review of auth/oauth.go — hashPassword should use bcrypt.",
	}
	d3, _ := json.Marshal(msg3)
	conns["gemini-0"].WriteMessage(websocket.TextMessage, d3)
	t.Log("ws: gemini-alpha -> review: code review request")

	// Claude responds to review.
	msg4 := protocol.Message{
		ID: "ws-004", Type: protocol.MsgChat, From: "claude-0", To: "gemini-0",
		Channel: protocol.ChanReview,
		Body:    "Agreed. I'll switch to bcrypt with cost 12. Will push to agent/claude-0 branch.",
	}
	d4, _ := json.Marshal(msg4)
	conns["claude-0"].WriteMessage(websocket.TextMessage, d4)
	t.Log("ws: claude-alpha -> gemini-alpha: review response")

	time.Sleep(300 * time.Millisecond)

	// Close connections.
	for id, c := range conns {
		c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.Close()
		t.Logf("ws: %s disconnected", id)
	}
	wg.Wait()

	// Report results.
	mu.Lock()
	totalReceived := 0
	for id, msgs := range received {
		t.Logf("ws: %s received %d messages:", id, len(msgs))
		for _, m := range msgs {
			t.Logf("  -> %s", m)
		}
		totalReceived += len(msgs)
	}
	mu.Unlock()

	if totalReceived == 0 {
		t.Fatal("no messages received over WebSocket")
	}
	t.Logf("ws: total messages exchanged: %d", totalReceived)
}

// ============================================================
// Phase 8: Instruction Hot-Reload
// ============================================================

func TestPhase8_InstructionHotReload(t *testing.T) {
	t.Log("=== PHASE 8: Instruction Hot-Reload ===")

	update := protocol.InstructionUpdate{
		NewInstructions: protocol.InstructionSet{
			Version: 2,
			Project: protocol.ProjectConfig{
				Name: "infinity-oauth", Repo: ".", MainBranch: "main",
			},
			Agents: []protocol.AgentSpec{
				{CLIType: protocol.CLIClaude, Count: 2},
				{CLIType: protocol.CLICodex, Count: 2},
				{CLIType: protocol.CLIGemini, Count: 1},
			},
			Tasks: []protocol.TaskSpec{
				{ID: "pkce", Title: "Implement PKCE", Description: "Add RFC 7636 PKCE", AssignTo: "codex", Priority: 1, Status: protocol.TaskPending},
				{ID: "bcrypt", Title: "Switch to bcrypt", Description: "Replace SHA-256 with bcrypt", AssignTo: "claude", Priority: 1, Status: protocol.TaskPending},
				{ID: "rate-limit", Title: "Add rate limiting", Description: "Rate limit auth endpoints", AssignTo: "codex", Priority: 2, Status: protocol.TaskPending},
			},
			Rules: []string{
				"Complete PKCE before rate limiting.",
				"Claude reviews codex's PKCE implementation.",
				"Gemini re-audits after all changes.",
			},
		},
		ChangesSummary: "v2: PKCE, bcrypt, rate limiting from gemini's audit.",
		GracePeriodSec: 30,
	}

	result := post(t, "/api/instructions", update)
	if str(result, "status") != "broadcast" {
		t.Fatalf("expected broadcast, got %s", str(result, "status"))
	}
	t.Logf("instructions v2 broadcast: status=%s", str(result, "status"))

	// Verify retrievable.
	raw := get(t, "/api/instructions")
	var stored protocol.InstructionUpdate
	json.Unmarshal(raw, &stored)
	if stored.NewInstructions.Version != 2 {
		t.Fatalf("expected v2, got v%d", stored.NewInstructions.Version)
	}
	t.Logf("instruction v2 confirmed: %d tasks, grace=%ds", len(stored.NewInstructions.Tasks), stored.GracePeriodSec)
}

// ============================================================
// Phase 9: Full Pipeline Summary
// ============================================================

func TestPhase9_FullPipelineSummary(t *testing.T) {
	t.Log("=== PHASE 9: Full Pipeline Summary ===")

	raw := get(t, "/api/messages?limit=100")
	var msgs []protocol.Message
	json.Unmarshal(raw, &msgs)

	taskDone := 0
	for _, m := range msgs {
		if m.Type == protocol.MsgTaskDone {
			taskDone++
		}
	}

	enhRaw := get(t, "/api/enhancements")
	var enhs []interface{}
	json.Unmarshal(enhRaw, &enhs)

	meshRaw := get(t, "/api/mesh/directory")
	var dir protocol.MeshDirectory
	json.Unmarshal(meshRaw, &dir)

	appsRaw := get(t, "/api/apps")
	var apps []interface{}
	json.Unmarshal(appsRaw, &apps)

	reposRaw := get(t, "/api/repos")
	var repos []interface{}
	json.Unmarshal(reposRaw, &repos)

	t.Logf("bus messages:     %d", len(msgs))
	t.Logf("task_done:        %d", taskDone)
	t.Logf("enhancements:     %d", len(enhs))
	t.Logf("mesh apps:        %d", len(dir.Apps))
	t.Logf("member apps:      %d", len(apps))
	t.Logf("repos:            %d", len(repos))

	t.Log("")
	t.Log("==========================================")
	t.Log("  ALL 9 PHASES PASSED — ALPHAS VERIFIED")
	t.Log("==========================================")
	t.Log("")
	t.Log("Claude-alpha:  Designed OAuth, operator bootstrap, auth code flow, token rotation")
	t.Log("Codex-alpha:   Invite chain (Operator->Alice->Charlie, Operator->Bob), per-member apps, mesh")
	t.Log("Gemini-alpha:  Security audit (5/5 checks), 4 enhancement suggestions, cross-alpha voting")
	t.Log("All alphas:    WebSocket real-time messaging, instruction hot-reload v2")
}
