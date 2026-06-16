package handlers

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
	"github.com/opc/api/internal/middleware"
)

// textClientResolver is the subset of *agents.ProviderResolver
// that handlers consume. Defining it locally keeps the handler
// unit tests free of a *gorm.DB and lets us pass a stub in
// tests.
type textClientResolver interface {
	Text(ctx context.Context, userID uint, scope string) (agents.TextProvider, error)
}

// resolveTextForRequest returns the text provider to use for the
// current request and stamps the call_log ref (when present) with
// the resolved provider name so the observability dashboard can
// show which protocol adapter served the call. The lookup chain is:
//
//  1. The resolver's per-request pick (uses user_id + scope from
//     the caller's session cookie).
//  2. The handler's configured default (passed via the constructor
//     — typically a NewXxxWithTextOverride shim during tests).
//  3. The package-level Echo (always non-nil) — guarantees the
//     handler never panics on a nil resolver or nil default.
//
// The provider name is stamped only when the resolver (or default)
// actually resolves a provider — when we fall through to Echo, the
// middleware's default of "internal" stays in place because Echo
// is a deterministic stub, not a billable upstream.
func resolveTextForRequest(c *gin.Context, r textClientResolver, def agents.TextProvider) agents.TextProvider {
	if r != nil {
		userID := UserIDFromContext(c)
		if userID != 0 {
			if p, err := r.Text(c.Request.Context(), userID, "all"); err == nil && p != nil {
				stampCallLogProvider(c, p.Name())
				return p
			}
		}
	}
	if def != nil {
		// Only stamp the default path when it differs from Echo,
		// to keep the row consistent with the resolver branch.
		if def.Name() != "echo" {
			stampCallLogProvider(c, def.Name())
		}
		return def
	}
	return agents.Echo
}

// stampCallLogProvider sets ref.Provider on the in-flight call_log
// row when the middleware is in the chain. A nil ref is a no-op —
// routes mounted outside the /api group (e.g. /api/ai/topics before
// Phase 4 fixed its chain) simply do not get a row.
func stampCallLogProvider(c *gin.Context, name string) {
	if c == nil {
		return
	}
	if ref := middleware.CallLogFromContext(c); ref != nil {
		ref.Provider = name
	}
}
