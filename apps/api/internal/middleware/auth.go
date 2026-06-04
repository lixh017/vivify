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

// RequireAuth returns a Gin middleware that resolves the session
// cookie via auth.ValidateSession and stashes the user on the Gin
// context. On any failure path it short-circuits with 401 — we never
// let an unauthenticated request through to a handler that expects a
// user, and we never echo the underlying error to the client (the
// body is always a generic "not authenticated" so the endpoint cannot
// be used to distinguish "no cookie" from "stale cookie" from
// "expired session" from "user row deleted").
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
			unauthorized(c)
			return
		}
		u, err := auth.ValidateSession(db, token)
		if err != nil {
			// We log the sentinel distinction server-side so the
			// operator can tell "client is unauthenticated" from
			// "client sent a tampered/expired token". The body is
			// intentionally identical.
			switch {
			case errors.Is(err, auth.ErrSessionExpired):
				logger.Info("require auth: session expired", "request_id", c.GetString("request_id"))
			case errors.Is(err, auth.ErrSessionNotFound):
				logger.Info("require auth: session not found", "request_id", c.GetString("request_id"))
			default:
				logger.Error("require auth: validate session", "err", err.Error(), "request_id", c.GetString("request_id"))
			}
			unauthorized(c)
			return
		}
		c.Set(CtxUserID, u.ID)
		c.Set(CtxUser, u)
		c.Next()
	}
}

// unauthorized writes the canonical 401 body. Centralized so the
// RequireAuth middleware and any future handler-level guard produce
// byte-identical responses (a small but useful property for clients
// and tests).
func unauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": "not authenticated",
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
