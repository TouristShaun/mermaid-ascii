package auth

import (
	"net/http"
	"strings"
)

// TokenMiddleware extracts and validates OAuth bearer tokens from requests.
// It injects the validated token info into the request context.
type TokenMiddleware struct {
	auth *Service
}

// NewTokenMiddleware creates middleware backed by the given auth service.
func NewTokenMiddleware(auth *Service) *TokenMiddleware {
	return &TokenMiddleware{auth: auth}
}

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const (
	ContextMemberID contextKey = "member_id"
	ContextAppID    contextKey = "app_id"
	ContextScopes   contextKey = "scopes"
)

// RequireAuth returns an HTTP middleware that enforces authentication.
func (tm *TokenMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		token, err := tm.auth.ValidateToken(parts[1])
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnauthorized)
			return
		}

		// Inject token info into context via headers (simple approach).
		r.Header.Set("X-Member-ID", token.MemberID)
		r.Header.Set("X-App-ID", token.AppID)
		r.Header.Set("X-Scopes", strings.Join(token.Scopes, ","))

		next.ServeHTTP(w, r)
	})
}

// RequireScope returns middleware that checks for a specific scope.
func (tm *TokenMiddleware) RequireScope(scope string, next http.Handler) http.Handler {
	return tm.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scopes := strings.Split(r.Header.Get("X-Scopes"), ",")
		for _, s := range scopes {
			if s == scope || s == "admin" {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, `{"error":"insufficient scope"}`, http.StatusForbidden)
	}))
}
