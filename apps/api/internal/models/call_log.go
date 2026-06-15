package models

import "time"

// CallLog records a single API invocation for the observability
// surface. Every protected /api/* request writes one row (with a few
// exceptions — /api/auth/*, /api/health, /api/observability/* — see
// middleware/call_log.go). The schema is intentionally append-only
// (no updated_at): the row is a fact about one HTTP call, and editing
// it after the fact would be an audit-trail violation.
//
// Why cents not floats: cost computations accumulate across many
// rows, and floating-point error on small per-call amounts (sub-cent)
// would eventually drift reconciliation against the provider invoice.
// Storing cost in integer cents is the same discipline used by Stripe
// and most billing systems; arithmetic stays exact.
//
// The cost fields are reserved for Phase 4 (issue #46, billing
// audit). Today's call_log middleware writes cost=0 for every row —
// the columns exist now so the schema does not have to migrate when
// the billing phase lands, and so the /observability/summary endpoint
// can sum a non-zero column without a UNION against a placeholder.
type CallLog struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// UserID is the owner of the call. Phase 2 multi-tenant — every
	// aggregated query (by_skill, by_provider, today) filters on
	// this column. Indexed because the dashboard query is the hot
	// read path.
	//
	// Sub-Spec D M1: a row is owned by either a user (UserID != 0,
	// AgentID == 0, ActorType == "human") or an agent (UserID == 0,
	// AgentID != 0, ActorType == "agent"). The two columns are
	// intentionally disjoint — the middleware enforces this so a
	// misconfigured chain cannot write both, and a misconfigured
	// handler cannot leak across the boundary.
	UserID uint `gorm:"index" json:"user_id"`

	// AgentID is the owner of the call when the caller is a
	// non-human agent authenticated via X-API-Key. Sub-Spec D M1
	// adds this column. Indexed so the future "show me all calls
	// from this agent" dashboard query is a cheap lookup. Zero
	// for human-originated calls.
	AgentID uint `gorm:"index" json:"agent_id"`

	// ActorType is "human" or "agent" — the discriminator that
	// tells the operator which side of the Sub-Spec D boundary
	// produced this row. Default "human" matches every
	// pre-Sub-Spec-D row, so AutoMigrate on an existing DB leaves
	// the existing aggregate queries unchanged. A future
	// "by_actor" rollup view can GROUP BY this column directly.
	ActorType string `gorm:"size:16;default:human;index" json:"actor_type"`

	// Skill is a short, stable identifier for the capability the
	// caller invoked. Conventions:
	//   - "ai_topics"        — POST /api/ai/topics
	//   - "ai_score"         — POST /api/ai/score
	//   - "cover_generate"   — POST /api/cover
	//   - "credentials_list" — GET  /api/credentials
	//   - "topics_list"      — GET  /api/topics
	// The middleware infers it from the URL path; handlers that want
	// a more specific value can override the field on the
	// CallLogRef before the middleware finalizes the row.
	Skill string `gorm:"size:64;index" json:"skill"`

	// Provider identifies the upstream service that produced the
	// response. "internal" is the default for handlers that do not
	// touch an LLM or third-party API; the cover handler overrides
	// to "volcengine", the AI handler overrides to "anthropic" /
	// "deepseek" / "gemini" based on the resolved route. The column
	// is indexed because the dashboard groups cost by provider.
	Provider string `gorm:"size:32;index" json:"provider"`

	// CredentialID is a soft reference to the credentials row that
	// was used for this call. It is a *uint because not every call
	// uses a credential (e.g. an internal handler that talks to
	// SQLite needs no API key). Indexed so the future
	// "rotate this credential — how many calls in the last 7 days"
	// view is a cheap lookup.
	CredentialID *uint `gorm:"index" json:"credential_id,omitempty"`

	// Status is a coarse classification of the response. The
	// mapping from HTTP code lives in the middleware:
	//   0 = 2xx      (ok)
	//   1 = 4xx      (client error)
	//   2 = 5xx      (server error)
	//   3 = timeout  (request did not complete)
	// Storing the bucket (not the raw code) keeps the by_skill
	// success_rate aggregation as a SUM(status=0)/COUNT(*) over a
	// small integer column, which the query planner can serve from
	// the UserID index without scanning every row.
	Status int `gorm:"index" json:"status"`

	// LatencyMS is wall-clock from middleware entry to
	// response-write completion. Stored in milliseconds as a plain
	// int so the p50/p95 query (see handlers/observability.go) can
	// use SQLite's percentiles over an integer column.
	LatencyMS int `json:"latency_ms"`

	// InputTokens / OutputTokens are LLM usage counters. Zero for
	// non-LLM calls; the AI handlers will fill these in once the
	// usage-seam lands in Phase 4. The columns exist now so the
	// schema does not migrate later.
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`

	// CostCents is the per-call cost in INTEGER cents. The cost
	// table in internal/config/cost.go is the source of truth for
	// the unit price; the handler multiplies and stores the result
	// here. Storing the result (not recomputing on read) means a
	// pricing change does not retroactively rewrite history.
	CostCents int `json:"cost_cents"`

	// CostUnits is the count of provider-billed units that went
	// into this call (chars for TTS, tokens for text, image
	// count for cover, etc.). Storing it separately from
	// CostCents lets the operator re-derive "what would this
	// have cost under today's pricing" for invoice
	// reconciliation — a future Phase 4 billing feature
	// (#46 audit: "改一次价历史 call_log 的 cost 含义全变")
	// uses this field to recompute without mutating the
	// historical row.
	CostUnits int `json:"cost_units"`

	// CostCurrency is a 3-letter ISO code. Default "CNY" matches
	// the provider invoice currency (volcengine bills in CNY;
	// anthropic in USD — when a USD call is logged, the handler
	// should override the default at write time, not on read).
	CostCurrency string `gorm:"size:8;default:CNY" json:"cost_currency"`

	// ErrorMessage is set on non-ok responses so the operator can
	// jump from "5xx spike on ai_topics" to a representative
	// message without correlating request_id. Empty on success.
	ErrorMessage string `gorm:"type:text" json:"error_message,omitempty"`

	// CreatedAt is when the middleware observed the response.
	// Indexed because every dashboard query orders by it.
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// TableName pins the underlying SQLite table name so the model is
// unambiguous in logs, migrations, and the migrate tests' table list.
func (CallLog) TableName() string { return "call_logs" }

// CallStatusOk / ClientError / ServerError / Timeout are the
// status-bucket constants stored in CallLog.Status. Centralized so
// the middleware and the observability handler agree on the
// vocabulary and the by_skill success_rate computation can be a
// `WHERE status = CallStatusOk` rather than a fragile `status = 0`
// sprinkled across the codebase.
const (
	CallStatusOK          = 0
	CallStatusClientError = 1
	CallStatusServerError = 2
	CallStatusTimeout     = 3
)
