package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
)

// textClientResolver is the subset of *agents.ProviderResolver
// that handlers consume. Defining it locally keeps the handler
// unit tests free of a *gorm.DB and lets us pass a stub in
// tests.
type textClientResolver interface {
	Text(ctx context.Context, userID uint, scope string) (agents.TextProvider, error)
}

// resolveTextForRequest returns the text provider to use for the
// current request. The lookup chain is:
//
//  1. The resolver's per-request pick (uses user_id + scope from
//     the caller's session cookie).
//  2. The handler's configured default (passed via the constructor
//     — typically a NewXxxWithTextOverride shim during tests).
//  3. The package-level Echo (always non-nil) — guarantees the
//     handler never panics on a nil resolver or nil default.
func resolveTextForRequest(c *gin.Context, r textClientResolver, def agents.TextProvider) agents.TextProvider {
	if r != nil {
		userID := UserIDFromContext(c)
		if userID != 0 {
			if p, err := r.Text(c.Request.Context(), userID, "all"); err == nil && p != nil {
				return p
			}
		}
	}
	if def != nil {
		return def
	}
	return agents.Echo
}
