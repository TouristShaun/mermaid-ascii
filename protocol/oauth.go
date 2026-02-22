package protocol

import "time"

// OAuthConfig defines the referral-only OAuth system for Infinity Forward Ltd.
// No public registration. Every member is invited by an existing member or
// the system operator.

// Member represents a person who has opted in to a forever relationship
// with Infinity Forward Ltd.
type Member struct {
	ID           string    `json:"id"`
	DisplayName  string    `json:"display_name"`
	Email        string    `json:"email"`
	InvitedBy    string    `json:"invited_by"`    // member ID of who referred them
	InviteCode   string    `json:"invite_code"`   // the code they used to join
	JoinedAt     time.Time `json:"joined_at"`
	IsOperator   bool      `json:"is_operator"`   // the team-of-one founder
}

// MemberApp is a custom application instance created for a specific member.
// Each member gets their own app. Apps can communicate through the mesh.
type MemberApp struct {
	ID          string            `json:"id"`
	MemberID    string            `json:"member_id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	ClientID    string            `json:"client_id"`     // OAuth client_id
	Secret      string            `json:"secret"`        // hashed client_secret
	RedirectURI string            `json:"redirect_uri"`
	Scopes      []string          `json:"scopes"`
	MeshEnabled bool              `json:"mesh_enabled"`  // can talk to other member apps
	Config      map[string]string `json:"config,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// InviteCode is a single-use referral code created by an existing member.
type InviteCode struct {
	Code       string    `json:"code"`
	CreatedBy  string    `json:"created_by"`   // member ID
	UsedBy     string    `json:"used_by,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
	Used       bool      `json:"used"`
}

// OAuthToken is an issued access/refresh token pair.
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`   // always "Bearer"
	ExpiresAt    time.Time `json:"expires_at"`
	Scopes       []string  `json:"scopes"`
	MemberID     string    `json:"member_id"`
	AppID        string    `json:"app_id"`
}

// AuthorizationRequest is the OAuth authorize step.
type AuthorizationRequest struct {
	ClientID     string `json:"client_id"`
	RedirectURI  string `json:"redirect_uri"`
	Scope        string `json:"scope"`
	State        string `json:"state"`
	ResponseType string `json:"response_type"` // "code"
}

// TokenRequest is the OAuth token exchange step.
type TokenRequest struct {
	GrantType    string `json:"grant_type"`    // "authorization_code" or "refresh_token"
	Code         string `json:"code,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// Available OAuth scopes.
const (
	ScopeProfile    = "profile"      // read member profile
	ScopeApps       = "apps"         // manage own apps
	ScopeMesh       = "mesh"         // inter-app communication
	ScopeSuggest    = "suggest"      // send enhancement suggestions
	ScopeAdmin      = "admin"        // operator-only
)
