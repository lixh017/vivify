// Package middleware: agent_auth.go implements MaybeAgentKey, the
// X-API-Key header validator for non-human agent callers. It is the
// X-API-Key counterpart to RequireAuth (which handles the session
// cookie side): together they cover both call paths the Sub-Spec D
// spec requires handlers to accept.
package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// Context keys for the authenticated agent. Exported as constants
// so handlers and tests can read them without sprinkling
// stringly-typed keys across the codebase. Symmetric with the
// CtxUserID / CtxUserRole constants in auth.go.
const (
	// CtxAgentID is the uint Agent.ID of the calling agent. Handlers
	// should treat 0 (or a missing key) as "not authenticated".
	CtxAgentID = "agent_id"
	// CtxAgentScope is the Agent.Scope string ("all" today; reserved
	// for future narrower scopes). Handlers that need to gate on
	// scope can read this without re-querying the DB.
	CtxAgentScope = "agent_scope"
	// CtxAgentName is the human-readable Agent.Name. Useful for
	// logging the calling agent on /api/ai/* requests.
	CtxAgentName = "agent_name"
)

// keyPrefixLiteral is the literal prefix every agent key carries.
// Used to short-circuit malformed keys before the DB roundtrip.
const keyPrefixLiteral = "opc_agent_"

// keyHexLen is the number of random hex chars after the prefix.
// 32 hex chars = 16 bytes of crypto/rand entropy; matches the
// constant embedded in models.GenerateKey so a key and a row
// cannot drift out of agreement.
const keyHexLen = 32

// MaybeAgentKey inspects the X-API-Key request header. If present
// and valid, it stamps c.Set("agent_id", <id>),
// c.Set("agent_scope", <scope>), and c.Set("agent_name", <name>)
// on the Gin context. If missing, malformed, disabled, or unknown
// the middleware PASSES THROUGH without aborting — it composes
// with RequireAuth in the middleware chain, and the handler
// decides whether either path is sufficient by checking
// c.GetUint("user_id") > 0 || c.Get("agent_id") != nil.
//
// Use this AFTER RequireAuth in the middleware chain so that
// session-based human callers without X-API-Key are not affected:
//
//	r.GET("/api/ai/topics",
//	    middleware.RequireAuth(db, logger),
//	    middleware.MaybeAgentKey(db),
//	    handler.GenerateTopics)
//
// Rationale for pass-through rather than 401: an agent caller
// might also carry a session cookie (admin acting as agent); an
// invalid key should not deny access the cookie would otherwise
// grant. Symmetric with the other side: a human caller without a
// cookie would also be denied by the handler's own auth check,
// not by MaybeAgentKey.
//
// The lookup is a single prefix-indexed query (no
// per-character scanning) followed by a single bcrypt compare per
// candidate. Disabling is honored at the candidate-skip step so a
// revoked key cannot auth even if its bcrypt still matches.
func MaybeAgentKey(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-API-Key")
		if key == "" {
			c.Next()
			return
		}
		// Format check before the DB roundtrip. The key shape is
		// "opc_agent_" + 32 hex chars (see models.GenerateKey).
		// A wrong prefix or wrong length is a guarantee of
		// "no match" — skip the query.
		if !strings.HasPrefix(key, keyPrefixLiteral) || len(key) != len(keyPrefixLiteral)+keyHexLen {
			c.Next()
			return
		}
		prefix := models.KeyPrefix(key)
		var candidates []models.Agent
		if err := db.Where("key_prefix = ?", prefix).Find(&candidates).Error; err != nil {
			// Graceful — let RequireAuth (above) or the handler's
			// own auth check emit a 401 with a proper reason code.
			c.Next()
			return
		}
		for _, a := range candidates {
			if a.Disabled {
				continue
			}
			// Single bcrypt compare per candidate. bcrypt is
			// already constant-time; no need for an explicit
			// subtle.ConstantTimeCompare. The prefix index keeps
			// the candidate set tiny (in practice 1).
			if !models.VerifyPassword(a.HashedKey, key) {
				continue
			}
			// Stamp last_used_at (fire-and-forget; a failure to
			// update the timestamp must not break the request).
			now := time.Now()
			db.Model(&a).Update("last_used_at", &now)

			c.Set(CtxAgentID, a.ID)
			c.Set(CtxAgentScope, a.Scope)
			c.Set(CtxAgentName, a.Name)
			c.Next()
			return
		}
		// No candidate matched (bcrypt failed for all). Pass
		// through; the handler's auth check will 401 if the
		// endpoint requires either auth path.
		c.Next()
	}
}
