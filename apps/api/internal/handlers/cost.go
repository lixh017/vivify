package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/config"
	"github.com/opc/api/internal/middleware"
)

// StampTextCost fills in Provider / InputTokens / OutputTokens /
// CostCents / CostUnits on the call_log row that the call_log
// middleware created for the current request. Ref is resolved
// from the gin context; a nil ref (e.g. when call_log middleware
// was skipped for this route) is a no-op so handlers can call
// this unconditionally.
//
// provider is the upstream service name (e.g. "minimax").
// The cost stamp keeps the operator's "what did this cost
// at the time" answer stable: CostCents is the cents paid,
// CostUnits is the count of billable units, and the skill
// string identifies the price table row. Together those three
// fields let the dashboard re-derive the "what would this
// have cost under today's pricing" without mutating the
// historical row (#46 audit: "改一次价历史 call_log 的 cost
// 含义全变").
func StampTextCost(c *gin.Context, provider string, skill config.Skill, in, out int) {
	ref := middleware.CallLogFromContext(c)
	if ref == nil {
		return
	}
	ref.Provider = provider
	ref.InputTokens = in
	ref.OutputTokens = out
	ref.CostCents = config.CostForTokenSkill(skill, in, out)
	// CostUnits is the total tokens; the audit can multiply
	// by today's price to recompute without trusting the
	// snapshot.
	ref.CostUnits = in + out
}

// StampFlatCost fills in Provider / CostCents / CostUnits for
// calls that have a per-call flat price (cover image
// generation, TTS per-call, video per-clip, etc.).
// InputTokens / OutputTokens are left at zero so the dashboard
// still distinguishes "per-call" rows from "per-token" rows.
func StampFlatCost(c *gin.Context, provider string, skill config.Skill, units int) {
	ref := middleware.CallLogFromContext(c)
	if ref == nil {
		return
	}
	ref.Provider = provider
	ref.CostCents = config.CostForSkill(skill, units)
	ref.CostUnits = units
}

// StampCostCents is the most direct variant: caller has already
// computed the cents (e.g. a config lookup that returned a
// non-table value) and just wants the call_log row stamped. Use
// this when neither token-based nor table-based pricing fits.
func StampCostCents(c *gin.Context, provider string, cents int) {
	ref := middleware.CallLogFromContext(c)
	if ref == nil {
		return
	}
	ref.Provider = provider
	ref.CostCents = cents
}

// StampClaudeCost is a back-compat alias for StampTextCost with
// provider="minimax" — the historical name from the Phase 3
// Claude era. New code should call StampTextCost directly so
// the provider argument is explicit. We keep the alias
// (rather than rename) so the existing 8 call sites in
// quality.go / pipeline.go / batch.go / deconstruct.go don't
// all need to change in one PR — a follow-up commit can
// rename + remove the alias.
func StampClaudeCost(c *gin.Context, skill config.Skill, in, out int) {
	StampTextCost(c, "minimax", skill, in, out)
}
