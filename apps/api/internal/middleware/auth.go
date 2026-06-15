// Package middleware: auth.go owns the session-cookie auth surface.
//
// Why this lives in the middleware package and not in handlers:
//
//  1. The skip-list the spec calls for ("/auth/*, /healthz, /readyz,
//     /openapi.json, /docs, /metrics") is enforced by route placement
//     in main.go, not by a string check inside the middleware — so
//     the middleware itself has no opinions about URL prefixes, and
//     placing it in middleware avoids a coupling that handlers does
//     not need.
//
//  2. handlers/middleware.go (which holds RequestID) is allowed to
//     stay handler-only: handlers/*_test.go imports the middleware
//     package for Metrics(), so a re-export back from handlers into
//     middleware would be an import cycle. Putting the auth
//     middleware (and the context keys it sets) in the middleware
//     package is the only way to break it.
package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/auth"
	"github.com/opc/api/internal/models"
)

// sessionCookieName is the cookie the browser sends. Centralized so
// the auth handler and the middleware agree on the spelling — a typo
// in one place would silently 401 every request.
const sessionCookieName = "opc_session"

// Context keys for the authenticated user. They are exported as
// constants so downstream handlers (and tests) can pull the current
// user off the Gin context without sprinkling stringly-typed keys
// across the codebase.
const (
	// CtxUserID is the uint user ID. Handlers should read this with
	// UserIDFromContext. 0 means "not authenticated" — every
	// RequireAuth-protected handler should assert it is non-zero
	// before doing per-user work.
	CtxUserID = "user_id"
	// CtxUserRole is the user's role string ("operator" or
	// "creator"). Populated by RequireAuth from the loaded User
	// row so downstream role-check middleware
	// (RequireOperatorRole / RequireCreatorRole) can read it
	// without re-querying the DB.
	CtxUserRole = "user_role"
	// CtxUser is the full models.User value. Useful for handlers
	// that need the email or name (e.g. /api/auth/me-like endpoints
	// that look up the caller's profile).
	CtxUser = "user"
)

// sessionTokenFromCookie reads the session token from the
// request cookie. Returns "" if the cookie is missing — the caller
// (RequireAuth / Me) maps that to 401.
func sessionTokenFromCookie(c *gin.Context) string {
	v, err := c.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return v
}

// Canonical reason codes for 401 responses. Exposed so handlers (and
// tests) can build payloads without sprinkling stringly-typed values
// around. The values are part of the API contract — changing them is
// a breaking change for any client that branches on `reason`.
const (
	// AuthReasonNoCookie means the request did not include the
	// opc_session cookie. Frontend: prompt the user to log in.
	AuthReasonNoCookie = "no_cookie"
	// AuthReasonInvalidSignature means the cookie was present but
	// the token does not correspond to any known session (likely
	// tampered, or from a different environment). Frontend: clear
	// the stale cookie and re-login.
	AuthReasonInvalidSignature = "invalid_signature"
	// AuthReasonExpired means the session row existed but its
	// ExpiresAt is in the past. Frontend: re-login to mint a fresh
	// session.
	AuthReasonExpired = "expired"
	// AuthReasonWrongScope means the session is valid but the user
	// is not allowed to call this endpoint. Reserved for future
	// scope/role checks; not currently produced by the middleware.
	AuthReasonWrongScope = "wrong_scope"
	// AuthReasonUnknownUser means the session row points at a user
	// that no longer exists. Frontend: clear the stale cookie and
	// re-login.
	AuthReasonUnknownUser = "unknown_user"
)

// authHint is a small UX nudge included with every 401 so a frontend
// dev (or a human) reading the response knows which cookie to set and
// where to obtain one. The exact login route may change; this string
// is a deliberate "first-step pointer" rather than a strict
// navigation target.
const authHint = "POST /api/auth/login to obtain an opc_session cookie"

// RequireAuth returns a Gin middleware that resolves the session
// cookie via auth.ValidateSession and stashes the user on the Gin
// context. On any failure path it short-circuits with 401 — we never
// let an unauthenticated request through to a handler that expects a
// user. The body is a generic "not authenticated" envelope plus a
// machine-readable `reason` so a frontend can decide whether to
// re-login (no_cookie / expired / unknown_user), retry with a
// refreshed request (transient), or surface a bug (invalid_signature
// from a non-malicious client). The underlying error itself is never
// echoed to the client, so the endpoint cannot be used as an oracle
// for which tokens are valid.
//
// The middleware takes a *gorm.DB explicitly (rather than reading
// from a package-level singleton) so tests can wire a custom DB. A
// nil db is a programmer error — we panic, matching the rest of the
// handlers package's "wire me at startup" idiom.
func RequireAuth(db *gorm.DB, logger *slog.Logger) gin.HandlerFunc {
	if db == nil {
		panic("middleware.RequireAuth: db is nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return func(c *gin.Context) {
		token := sessionTokenFromCookie(c)
		if token == "" {
			unauthorized(c, AuthReasonNoCookie)
			return
		}
		u, err := auth.ValidateSession(db, token)
		if err != nil {
			// We log the sentinel distinction server-side so the
			// operator can tell "client is unauthenticated" from
			// "client sent a tampered/expired token". The body's
			// `reason` is the machine-readable counterpart.
			reason := AuthReasonInvalidSignature
			switch {
			case errors.Is(err, auth.ErrSessionExpired):
				reason = AuthReasonExpired
				logger.Info("require auth: session expired", "request_id", c.GetString("request_id"))
			case errors.Is(err, auth.ErrSessionNotFound):
				// ValidateSession collapses "unknown token" and
				// "user row deleted" into the same sentinel; from
				// the client's perspective both are "your cookie
				// does not authorize you" and the right recovery is
				// identical (clear + re-login), so we label this
				// the more common case.
				reason = AuthReasonInvalidSignature
				logger.Info("require auth: session not found", "request_id", c.GetString("request_id"))
			default:
				reason = AuthReasonInvalidSignature
				logger.Error("require auth: validate session", "err", err.Error(), "request_id", c.GetString("request_id"))
			}
			unauthorized(c, reason)
			return
		}
		c.Set(CtxUserID, u.ID)
		c.Set(CtxUserRole, u.Role)
		c.Set(CtxUser, u)
		c.Next()
	}
}

// unauthorized writes the canonical 401 body. Centralized so the
// RequireAuth middleware and any future handler-level guard produce
// byte-identical responses (a small but useful property for clients
// and tests). The `reason` is a machine-readable code from the
// AuthReason* constants; the `hint` is a static pointer to the login
// endpoint so a frontend dev never has to dig through the OpenAPI to
// figure out where to obtain a session.
func unauthorized(c *gin.Context, reason string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error":  "not authenticated",
		"reason": reason,
		"hint":   authHint,
	})
}

// UserIDFromContext returns the authenticated user ID stored by
// RequireAuth. A return of 0 means "no user" — the caller is
// expected to have run RequireAuth (otherwise it would always see 0
// and produce a confusing 500 downstream).
func UserIDFromContext(c *gin.Context) uint {
	v, ok := c.Get(CtxUserID)
	if !ok {
		return 0
	}
	if id, ok := v.(uint); ok {
		return id
	}
	return 0
}

// UserFromContext returns the full user stored by RequireAuth. The
// second return is false if RequireAuth did not run (or did not
// resolve a session); handlers should treat that as an auth bug and
// 500 — RequireAuth is the only legitimate path that sets the value.
func UserFromContext(c *gin.Context) (models.User, bool) {
	v, ok := c.Get(CtxUser)
	if !ok {
		return models.User{}, false
	}
	u, ok := v.(models.User)
	return u, ok
}

// StubUser is a test-only middleware that stamps a synthetic
// CtxUserID onto the context. It deliberately does NOT touch
// CtxUser, so handlers that look up the full user (none today, but
// future ones) still get a "no user" signal. Production code MUST
// NOT use this — it exists so the per-entity handler unit tests can
// keep their existing shape (router → handler → response) without
// spinning up a session-cookie dance on every assertion.
//
// The guard introduced by Phase 2 Task 3 (requireUserID) means a
// test that forgets this middleware now fails loud with a 500
// rather than silently leaking across tenants — which is exactly
// the tripwire we wanted.
func StubUser(userID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(CtxUserID, userID)
		c.Next()
	}
}

// RequireOperatorRole returns a Gin middleware that verifies the
// authenticated user has role="operator" (the default role for OPC
// internal accounts). MUST be used AFTER RequireAuth, which is
// responsible for loading the user and stamping CtxUserRole on
// the Gin context. Without a prior RequireAuth the middleware
// short-circuits with 401 (no role in context).
//
// Returns 401 when CtxUserRole is missing (RequireAuth did not
// run) and 403 when the role is set but not "operator".
func RequireOperatorRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := c.Get(CtxUserRole)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no user role in context"})
			return
		}
		if role.(string) != "operator" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "operator role required"})
			return
		}
		c.Next()
	}
}

// RequireCreatorRole returns a Gin middleware that verifies the
// authenticated user has role="creator" (Phase 2 external beta
// test accounts). MUST be used AFTER RequireAuth, which is
// responsible for loading the user and stamping CtxUserRole on
// the Gin context.
//
// Returns 401 when CtxUserRole is missing (RequireAuth did not
// run) and 403 when the role is set but not "creator".
func RequireCreatorRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := c.Get(CtxUserRole)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no user role in context"})
			return
		}
		if role.(string) != "creator" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "creator role required"})
			return
		}
		c.Next()
	}
}
