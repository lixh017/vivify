package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
)

// providerHeader is the request-level hint that overrides
// the handler's default text provider. Operators use it to
// A/B-test providers per request, or to route a single
// skill through a different backend (e.g. "humanize this
// script with anthropic, not minimax") without changing
// the handler's default. The /sandbox UI exposes the same
// field as a dropdown.
//
// Resolution order:
//  1. The X-Text-Provider header (if non-empty and the
//     registry has a provider registered under that name)
//  2. The handler's configured default
//  3. The package-level Echo (always non-nil) — guarantees
//     the handler never panics on a nil registry
const providerHeader = "X-Text-Provider"

// resolveText returns the text provider to use for the
// current request. The lookup chain is documented on
// providerHeader above.
//
// Callers (the per-handler text methods) keep their own
// default field for the no-header case so the existing
// constructor signatures don't have to change. The
// ProviderSet is consulted only when a header is set.
//
// This helper is intentionally untyped: it returns
// agents.TextProvider, not a concrete type. Adding a new
// concrete provider (e.g. *AnthropicCompat) requires no
// changes here — register it in main.go and the header
// picks it up.
func resolveText(c *gin.Context, def agents.TextProvider, reg *agents.ProviderRegistry) agents.TextProvider {
	if reg != nil {
		if name := c.GetHeader(providerHeader); name != "" {
			if p, ok := reg.Text(name); ok && p != nil {
				return p
			}
		}
	}
	if def != nil {
		return def
	}
	return agents.Echo
}

// resolveMedia is the media equivalent. Currently always
// returns the default (media selection is not yet per-
// request — adding the X-Media-Provider header is a follow-up
// when the operator asks for it). The helper exists so the
// call sites stay uniform with resolveText.
func resolveMedia(c *gin.Context, def agents.MediaProvider, reg *agents.ProviderRegistry) agents.MediaProvider {
	// Per-request media provider selection is not yet wired;
	// the default Media is always used. The signature is
	// kept symmetric with resolveText so the future header
	// can be added here without touching call sites.
	_ = c
	if def != nil {
		return def
	}
	return nil
}
