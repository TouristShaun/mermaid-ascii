package protocol

import "time"

// MeshNetwork handles inter-app communication. Member apps can discover
// each other, send requests, and suggest enhancements through the mesh.

// MeshEndpoint is a capability that an app exposes to the mesh.
type MeshEndpoint struct {
	AppID       string   `json:"app_id"`
	Path        string   `json:"path"`        // e.g. "/api/data"
	Methods     []string `json:"methods"`     // GET, POST, etc.
	Description string   `json:"description"`
	Scopes      []string `json:"scopes"`      // required scopes to call
}

// MeshDirectory is the service registry for all connected member apps.
type MeshDirectory struct {
	Apps      map[string]*MeshAppEntry `json:"apps"`
	UpdatedAt time.Time                `json:"updated_at"`
}

// MeshAppEntry is one app's listing in the directory.
type MeshAppEntry struct {
	AppID       string          `json:"app_id"`
	MemberID    string          `json:"member_id"`
	MemberName  string          `json:"member_name"`
	Name        string          `json:"name"`
	Endpoints   []MeshEndpoint  `json:"endpoints"`
	Online      bool            `json:"online"`
	LastSeen    time.Time       `json:"last_seen"`
}

// Enhancement is a suggestion from one app/member to another.
type Enhancement struct {
	ID          string          `json:"id"`
	FromAppID   string          `json:"from_app_id"`
	ToAppID     string          `json:"to_app_id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Priority    EnhancePriority `json:"priority"`
	Status      EnhanceStatus   `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	Votes       []string        `json:"votes"` // member IDs who upvoted
}

// EnhancePriority levels.
type EnhancePriority string

const (
	EnhanceLow    EnhancePriority = "low"
	EnhanceMedium EnhancePriority = "medium"
	EnhanceHigh   EnhancePriority = "high"
)

// EnhanceStatus tracks the lifecycle of an enhancement suggestion.
type EnhanceStatus string

const (
	EnhanceProposed EnhanceStatus = "proposed"
	EnhanceAccepted EnhanceStatus = "accepted"
	EnhanceBuilding EnhanceStatus = "building"
	EnhanceShipped  EnhanceStatus = "shipped"
	EnhanceDeclined EnhanceStatus = "declined"
)

// MeshRequest is a cross-app API call routed through the forge.
type MeshRequest struct {
	ID        string            `json:"id"`
	FromApp   string            `json:"from_app"`
	ToApp     string            `json:"to_app"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`
	Token     string            `json:"token"` // OAuth bearer token
	Timestamp time.Time         `json:"timestamp"`
}

// MeshResponse is the reply to a MeshRequest.
type MeshResponse struct {
	RequestID  string            `json:"request_id"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
}
