package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// --- Admin aggregate endpoints ---

// adminStats is the response for /api/admin/stats.
type adminStats struct {
	Uptime         string         `json:"uptime"`
	Members        int            `json:"members"`
	Apps           int            `json:"apps"`
	Repos          int            `json:"repos"`
	Enhancements   int            `json:"enhancements"`
	InviteCodes    int            `json:"invite_codes"`
	ActiveTokens   int            `json:"active_tokens"`
	BusMessages    int            `json:"bus_messages"`
	MeshApps       int            `json:"mesh_apps"`
	MeshAppsOnline int            `json:"mesh_apps_online"`
	ConnectedWS    int            `json:"connected_ws"`
	EnhByStatus    map[string]int `json:"enhancements_by_status"`
	EnhByPriority  map[string]int `json:"enhancements_by_priority"`
}

var serverStartTime = time.Now()

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	stats := adminStats{
		Uptime:        time.Since(serverStartTime).Truncate(time.Second).String(),
		EnhByStatus:   make(map[string]int),
		EnhByPriority: make(map[string]int),
	}

	db := s.db.Conn()
	countRow(db, "SELECT COUNT(*) FROM members", &stats.Members)
	countRow(db, "SELECT COUNT(*) FROM member_apps", &stats.Apps)
	countRow(db, "SELECT COUNT(*) FROM repos", &stats.Repos)
	countRow(db, "SELECT COUNT(*) FROM enhancements", &stats.Enhancements)
	countRow(db, "SELECT COUNT(*) FROM invite_codes", &stats.InviteCodes)
	countRow(db, "SELECT COUNT(*) FROM oauth_tokens WHERE expires_at > datetime('now')", &stats.ActiveTokens)

	stats.BusMessages = len(s.hub.History("", 1000))
	stats.ConnectedWS = s.hub.ClientCount()

	dir := s.mesh.Directory()
	stats.MeshApps = len(dir.Apps)
	for _, app := range dir.Apps {
		if app.Online {
			stats.MeshAppsOnline++
		}
	}

	rows, err := db.Query(`SELECT status, priority, COUNT(*) FROM enhancements GROUP BY status, priority`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var st, pri string
			var c int
			rows.Scan(&st, &pri, &c)
			stats.EnhByStatus[st] += c
			stats.EnhByPriority[pri] += c
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleAdminMembers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Conn().Query(`
		SELECT m.id, m.display_name, m.email, COALESCE(m.invited_by,''), m.joined_at, m.is_operator,
		       (SELECT COUNT(*) FROM member_apps WHERE member_id = m.id) AS app_count,
		       (SELECT COUNT(*) FROM invite_codes WHERE created_by = m.id) AS invites_created,
		       (SELECT COUNT(*) FROM invite_codes WHERE created_by = m.id AND used = 1) AS invites_used
		FROM members m ORDER BY m.joined_at ASC`)
	if err != nil {
		http.Error(w, `{"error":"query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var members []map[string]interface{}
	for rows.Next() {
		var id, name, email, invitedBy string
		var joinedAt time.Time
		var isOp bool
		var appCount, invCreated, invUsed int
		rows.Scan(&id, &name, &email, &invitedBy, &joinedAt, &isOp, &appCount, &invCreated, &invUsed)
		members = append(members, map[string]interface{}{
			"id": id, "display_name": name, "email": email,
			"invited_by": invitedBy, "joined_at": joinedAt, "is_operator": isOp,
			"app_count": appCount, "invites_created": invCreated, "invites_used": invUsed,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(members)
}

func countRow(db *sql.DB, query string, dest *int) {
	db.QueryRow(query).Scan(dest)
}

// --- Cockpit HTML ---

func (s *Server) handleCockpit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(cockpitHTML))
}

const cockpitHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Forge — Admin Cockpit</title>
<style>
  :root {
    --bg: #0d1117; --surface: #161b22; --border: #30363d;
    --text: #e6edf3; --muted: #8b949e; --accent: #58a6ff;
    --green: #3fb950; --yellow: #d29922; --red: #f85149;
    --purple: #bc8cff; --orange: #f0883e;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { background: var(--bg); color: var(--text); font-family: 'SF Mono', 'Cascadia Code', 'Fira Code', monospace; font-size: 13px; }
  a { color: var(--accent); text-decoration: none; }

  .topbar { background: var(--surface); border-bottom: 1px solid var(--border); padding: 10px 20px; display: flex; align-items: center; justify-content: space-between; position: sticky; top: 0; z-index: 100; }
  .topbar h1 { font-size: 16px; font-weight: 600; }
  .topbar h1 span { color: var(--accent); }
  .topbar .status { display: flex; gap: 16px; align-items: center; }
  .topbar .dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; margin-right: 4px; }
  .dot.on { background: var(--green); box-shadow: 0 0 6px var(--green); }
  .dot.off { background: var(--red); }

  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(340px, 1fr)); gap: 12px; padding: 16px; }
  .card { background: var(--surface); border: 1px solid var(--border); border-radius: 8px; overflow: hidden; }
  .card-header { padding: 10px 14px; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; }
  .card-header h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 1px; color: var(--muted); }
  .card-header .badge { font-size: 11px; padding: 2px 8px; border-radius: 10px; background: var(--border); color: var(--text); }
  .card-body { padding: 12px 14px; max-height: 320px; overflow-y: auto; }
  .card-body::-webkit-scrollbar { width: 4px; }
  .card-body::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }

  .wide { grid-column: 1 / -1; }
  .card-body.bus { max-height: 400px; }

  .kv { display: flex; justify-content: space-between; padding: 4px 0; border-bottom: 1px solid var(--border); }
  .kv:last-child { border-bottom: none; }
  .kv .k { color: var(--muted); }
  .kv .v { font-weight: 600; }
  .kv .v.green { color: var(--green); }
  .kv .v.yellow { color: var(--yellow); }
  .kv .v.red { color: var(--red); }

  table { width: 100%; border-collapse: collapse; font-size: 12px; }
  th { text-align: left; color: var(--muted); font-weight: 500; padding: 4px 6px; border-bottom: 1px solid var(--border); }
  td { padding: 5px 6px; border-bottom: 1px solid var(--border); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 180px; }
  tr:last-child td { border-bottom: none; }
  tr:hover { background: rgba(88,166,255,0.05); }

  .tag { display: inline-block; padding: 1px 6px; border-radius: 4px; font-size: 10px; font-weight: 600; }
  .tag.claude { background: #58a6ff22; color: var(--accent); }
  .tag.codex { background: #3fb95022; color: var(--green); }
  .tag.gemini { background: #bc8cff22; color: var(--purple); }
  .tag.operator { background: #f0883e22; color: var(--orange); }
  .tag.alpha { background: #d2992222; color: var(--yellow); }
  .tag.high { background: #f8514922; color: var(--red); }
  .tag.medium { background: #d2992222; color: var(--yellow); }
  .tag.low { background: #3fb95022; color: var(--green); }
  .tag.proposed { background: #58a6ff22; color: var(--accent); }
  .tag.online { background: #3fb95022; color: var(--green); }
  .tag.offline { background: #f8514922; color: var(--red); }

  .msg { padding: 6px 8px; border-bottom: 1px solid var(--border); font-size: 12px; line-height: 1.5; }
  .msg:last-child { border-bottom: none; }
  .msg .ts { color: var(--muted); font-size: 10px; margin-right: 6px; }
  .msg .from { color: var(--accent); font-weight: 600; margin-right: 4px; }
  .msg .mtype { color: var(--purple); margin-right: 6px; font-size: 10px; }
  .msg .body { color: var(--text); }

  .empty { color: var(--muted); font-style: italic; text-align: center; padding: 20px; }

  @keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.5; } }
  .loading { animation: pulse 1.5s infinite; }
</style>
</head>
<body>

<div class="topbar">
  <h1><span>&#9881;</span> Forge Admin Cockpit</h1>
  <div class="status">
    <span id="ws-status"><span class="dot off"></span> connecting...</span>
    <span id="uptime" style="color:var(--muted)">—</span>
    <span style="color:var(--muted)" id="clock"></span>
  </div>
</div>

<div class="grid">
  <!-- Stats Overview -->
  <div class="card" id="stats-card">
    <div class="card-header"><h2>System Overview</h2><span class="badge" id="health-badge">...</span></div>
    <div class="card-body" id="stats-body"><div class="empty loading">Loading...</div></div>
  </div>

  <!-- Agents -->
  <div class="card">
    <div class="card-header"><h2>Agents</h2><span class="badge" id="agent-count">0</span></div>
    <div class="card-body" id="agents-body"><div class="empty loading">Loading...</div></div>
  </div>

  <!-- Members -->
  <div class="card">
    <div class="card-header"><h2>Members</h2><span class="badge" id="member-count">0</span></div>
    <div class="card-body" id="members-body"><div class="empty loading">Loading...</div></div>
  </div>

  <!-- Apps -->
  <div class="card">
    <div class="card-header"><h2>Apps</h2><span class="badge" id="app-count">0</span></div>
    <div class="card-body" id="apps-body"><div class="empty loading">Loading...</div></div>
  </div>

  <!-- Mesh Network -->
  <div class="card">
    <div class="card-header"><h2>Mesh Network</h2><span class="badge" id="mesh-count">0</span></div>
    <div class="card-body" id="mesh-body"><div class="empty loading">Loading...</div></div>
  </div>

  <!-- Enhancements -->
  <div class="card">
    <div class="card-header"><h2>Enhancements</h2><span class="badge" id="enh-count">0</span></div>
    <div class="card-body" id="enh-body"><div class="empty loading">Loading...</div></div>
  </div>

  <!-- Repos -->
  <div class="card">
    <div class="card-header"><h2>Repositories</h2><span class="badge" id="repo-count">0</span></div>
    <div class="card-body" id="repos-body"><div class="empty loading">Loading...</div></div>
  </div>

  <!-- Instructions -->
  <div class="card">
    <div class="card-header"><h2>Instructions</h2><span class="badge" id="instr-badge">—</span></div>
    <div class="card-body" id="instr-body"><div class="empty loading">Loading...</div></div>
  </div>

  <!-- Live Bus -->
  <div class="card wide">
    <div class="card-header"><h2>Live Bus Feed</h2><span class="badge" id="bus-count">0</span></div>
    <div class="card-body bus" id="bus-body"><div class="empty">Waiting for messages...</div></div>
  </div>
</div>

<script>
const BASE = location.origin;
const WS_URL = (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws?agent_id=cockpit-admin';

// --- Helpers ---
function $(id) { return document.getElementById(id); }
function short(s, n) { return s && s.length > (n||12) ? s.slice(0, n||12) + '...' : s || '—'; }
function shortID(s) { return s ? s.slice(0,8) : '—'; }
function tag(cls, text) { return '<span class="tag ' + cls + '">' + text + '</span>'; }
function esc(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
function fmtTime(t) {
  if (!t) return '—';
  const d = new Date(t);
  return d.toLocaleTimeString('en-US', {hour12: false});
}

// --- Fetch JSON ---
async function api(path) {
  try { const r = await fetch(BASE + path); return await r.json(); }
  catch(e) { return null; }
}

// --- Render stats ---
async function loadStats() {
  const s = await api('/api/admin/stats');
  if (!s) { $('stats-body').innerHTML = '<div class="empty">Offline</div>'; return; }
  $('health-badge').textContent = 'healthy';
  $('health-badge').style.background = '#3fb95033';
  $('health-badge').style.color = '#3fb950';
  $('uptime').textContent = 'up ' + s.uptime;
  $('stats-body').innerHTML = [
    kv('Members', s.members),
    kv('Apps', s.apps),
    kv('Repos', s.repos),
    kv('Bus Messages', s.bus_messages),
    kv('WebSocket Clients', s.connected_ws, s.connected_ws > 0 ? 'green' : ''),
    kv('Mesh Apps', s.mesh_apps + ' (' + s.mesh_apps_online + ' online)'),
    kv('Active Tokens', s.active_tokens),
    kv('Invite Codes', s.invite_codes),
    kv('Enhancements', s.enhancements),
  ].join('');
}

function kv(k, v, cls) {
  return '<div class="kv"><span class="k">' + k + '</span><span class="v ' + (cls||'') + '">' + v + '</span></div>';
}

// --- Render agents ---
async function loadAgents() {
  const msgs = await api('/api/agents');
  if (!msgs || !msgs.length) { $('agents-body').innerHTML = '<div class="empty">No agents registered</div>'; $('agent-count').textContent = '0'; return; }
  const agents = [];
  const seen = {};
  msgs.forEach(function(m) {
    if (m.type === 'agent_join' && m.metadata && m.metadata.agent_id && !seen[m.metadata.agent_id]) {
      seen[m.metadata.agent_id] = true;
      agents.push(m);
    }
  });
  $('agent-count').textContent = agents.length;
  if (!agents.length) { $('agents-body').innerHTML = '<div class="empty">No agents</div>'; return; }
  let html = '<table><tr><th>Name</th><th>Type</th><th>Role</th></tr>';
  agents.forEach(function(m) {
    const cli = m.metadata.cli_type || '?';
    const alpha = m.metadata.is_alpha === 'true';
    html += '<tr><td>' + esc(m.body.split(' ')[0]) + '</td>';
    html += '<td>' + tag(cli, cli) + '</td>';
    html += '<td>' + (alpha ? tag('alpha', 'ALPHA') : 'member') + '</td></tr>';
  });
  html += '</table>';
  $('agents-body').innerHTML = html;
}

// --- Render members ---
async function loadMembers() {
  const members = await api('/api/admin/members');
  if (!members || !members.length) { $('members-body').innerHTML = '<div class="empty">No members</div>'; $('member-count').textContent = '0'; return; }
  $('member-count').textContent = members.length;
  let html = '<table><tr><th>Name</th><th>Email</th><th>Role</th><th>Apps</th><th>Invites</th></tr>';
  members.forEach(function(m) {
    html += '<tr><td title="' + m.id + '">' + esc(m.display_name) + '</td>';
    html += '<td>' + esc(m.email) + '</td>';
    html += '<td>' + (m.is_operator ? tag('operator', 'OPERATOR') : 'member') + '</td>';
    html += '<td>' + m.app_count + '</td>';
    html += '<td>' + m.invites_used + '/' + m.invites_created + '</td></tr>';
  });
  html += '</table>';
  $('members-body').innerHTML = html;
}

// --- Render apps ---
async function loadApps() {
  const apps = await api('/api/apps');
  if (!apps || !apps.length) { $('apps-body').innerHTML = '<div class="empty">No apps</div>'; $('app-count').textContent = '0'; return; }
  $('app-count').textContent = apps.length;
  let html = '<table><tr><th>Name</th><th>Client ID</th><th>Mesh</th></tr>';
  apps.forEach(function(a) {
    html += '<tr><td>' + esc(a.name) + '</td>';
    html += '<td title="' + a.client_id + '">' + short(a.client_id, 18) + '</td>';
    html += '<td>' + (a.mesh_enabled ? tag('online', 'ON') : tag('offline', 'OFF')) + '</td></tr>';
  });
  html += '</table>';
  $('apps-body').innerHTML = html;
}

// --- Render mesh ---
async function loadMesh() {
  const dir = await api('/api/mesh/directory');
  if (!dir || !dir.apps || !Object.keys(dir.apps).length) { $('mesh-body').innerHTML = '<div class="empty">No mesh apps</div>'; $('mesh-count').textContent = '0'; return; }
  const apps = Object.values(dir.apps);
  $('mesh-count').textContent = apps.length;
  let html = '<table><tr><th>App</th><th>Member</th><th>Endpoints</th><th>Status</th></tr>';
  apps.forEach(function(a) {
    html += '<tr><td>' + esc(a.name) + '</td>';
    html += '<td>' + esc(a.member_name) + '</td>';
    html += '<td>' + (a.endpoints ? a.endpoints.length : 0) + '</td>';
    html += '<td>' + (a.online ? tag('online', 'ONLINE') : tag('offline', 'OFFLINE')) + '</td></tr>';
  });
  html += '</table>';
  $('mesh-body').innerHTML = html;
}

// --- Render enhancements ---
async function loadEnhancements() {
  const enhs = await api('/api/enhancements');
  if (!enhs || !enhs.length) { $('enh-body').innerHTML = '<div class="empty">No enhancements</div>'; $('enh-count').textContent = '0'; return; }
  $('enh-count').textContent = enhs.length;
  let html = '<table><tr><th>Title</th><th>Priority</th><th>Status</th></tr>';
  enhs.forEach(function(e) {
    html += '<tr><td title="' + esc(e.description) + '">' + esc(short(e.title, 30)) + '</td>';
    html += '<td>' + tag(e.priority, e.priority.toUpperCase()) + '</td>';
    html += '<td>' + tag(e.status, e.status) + '</td></tr>';
  });
  html += '</table>';
  $('enh-body').innerHTML = html;
}

// --- Render repos ---
async function loadRepos() {
  const repos = await api('/api/repos');
  if (!repos || !repos.length) { $('repos-body').innerHTML = '<div class="empty">No repos</div>'; $('repo-count').textContent = '0'; return; }
  $('repo-count').textContent = repos.length;
  let html = '<table><tr><th>Name</th><th>Description</th><th>Created</th></tr>';
  repos.forEach(function(r) {
    html += '<tr><td>' + esc(r.name) + '</td>';
    html += '<td>' + esc(short(r.description, 30)) + '</td>';
    html += '<td>' + fmtTime(r.created_at) + '</td></tr>';
  });
  html += '</table>';
  $('repos-body').innerHTML = html;
}

// --- Render instructions ---
async function loadInstructions() {
  const data = await api('/api/instructions');
  if (!data || data.error) { $('instr-body').innerHTML = '<div class="empty">No instructions loaded</div>'; return; }
  const instr = data.new_instructions || data;
  $('instr-badge').textContent = 'v' + (instr.version || '?');
  let html = '';
  if (instr.project) {
    html += kv('Project', instr.project.name || '—');
    html += kv('Repo', instr.project.repo || '—');
    html += kv('Main Branch', instr.project.main_branch || '—');
  }
  if (instr.agents) {
    html += kv('Agent Specs', instr.agents.length + ' type(s)');
  }
  if (instr.tasks) {
    html += kv('Tasks', instr.tasks.length);
    instr.tasks.forEach(function(t) {
      html += '<div class="kv"><span class="k" style="padding-left:12px">' + esc(t.title) + '</span><span class="v">' + (t.status || 'pending') + '</span></div>';
    });
  }
  if (instr.rules && instr.rules.length) {
    html += kv('Rules', instr.rules.length);
  }
  $('instr-body').innerHTML = html || '<div class="empty">Empty instruction set</div>';
}

// --- Live bus via WebSocket ---
let busMessages = [];
let ws;
function connectWS() {
  ws = new WebSocket(WS_URL);
  ws.onopen = function() {
    $('ws-status').innerHTML = '<span class="dot on"></span> live';
  };
  ws.onclose = function() {
    $('ws-status').innerHTML = '<span class="dot off"></span> disconnected';
    setTimeout(connectWS, 3000);
  };
  ws.onerror = function() { ws.close(); };
  ws.onmessage = function(evt) {
    try {
      const msg = JSON.parse(evt.data);
      busMessages.unshift(msg);
      if (busMessages.length > 200) busMessages = busMessages.slice(0, 200);
      renderBus();
      $('bus-count').textContent = busMessages.length;
    } catch(e) {}
  };
}

function renderBus() {
  const body = $('bus-body');
  let html = '';
  busMessages.forEach(function(m) {
    html += '<div class="msg">';
    html += '<span class="ts">' + fmtTime(m.timestamp) + '</span>';
    html += '<span class="mtype">[' + esc(m.type) + ']</span>';
    html += '<span class="from">' + esc(m.from) + '</span> ';
    if (m.to && m.to !== 'all') html += '&rarr; ' + esc(m.to) + ' ';
    html += '<span class="body">' + esc(short(m.body, 80)) + '</span>';
    html += '</div>';
  });
  body.innerHTML = html || '<div class="empty">Waiting for messages...</div>';
}

// --- Load historical bus messages ---
async function loadBusHistory() {
  const msgs = await api('/api/messages?limit=50');
  if (msgs && msgs.length) {
    busMessages = msgs;
    renderBus();
    $('bus-count').textContent = busMessages.length;
  }
}

// --- Clock ---
function updateClock() {
  $('clock').textContent = new Date().toLocaleTimeString('en-US', {hour12: false});
}

// --- Init ---
async function init() {
  updateClock();
  setInterval(updateClock, 1000);
  await Promise.all([loadStats(), loadAgents(), loadMembers(), loadApps(), loadMesh(), loadEnhancements(), loadRepos(), loadInstructions(), loadBusHistory()]);
  connectWS();
  setInterval(function() { loadStats(); }, 5000);
  setInterval(function() { loadAgents(); loadMembers(); loadApps(); loadMesh(); loadEnhancements(); loadRepos(); }, 15000);
}
init();
</script>
</body>
</html>`
