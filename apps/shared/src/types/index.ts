// TypeScript types matching Go GORM models in apps/api/internal/models/*.
// JSON tags on the Go side use snake_case, so the wire format uses snake_case.
// We deliberately do NOT camelCase these fields: doing so would force an
// extra translation layer at every call site.

export type Topic = {
  id: number
  title: string
  angle: string
  platform: string
  status: string
  series_id?: number
  expected_performance: string
  notes: string
  created_at: string
  updated_at: string
}

export type Script = {
  id: number
  topic_id: number
  title: string
  content: string
  platform: string
  style_fingerprint: string
  tags: string
  word_count: number
  created_at: string
  updated_at: string
}

export type ContentItem = {
  id: number
  script_id: number
  platform: string
  // `null` means the date is cleared (e.g. back to 待定 on the calendar);
  // `undefined` means the field is absent. The Go server returns `null`
  // for cleared fields, so we model that explicitly rather than collapsing
  // the two states.
  scheduled_at?: string | null
  published_at?: string | null
  platform_url: string
  performance_metrics: string
  created_at: string
}

// PATCH payload for /content-items/:id. Each field is independently
// optional, and the nullable timestamp fields can be explicitly cleared
// by sending `null`. This matches the Go handler's Update semantics.
export type UpdateContentItemPatch = {
  platform?: string
  scheduled_at?: string | null
  published_at?: string | null
  platform_url?: string
  performance_metrics?: string
}

export type KnowledgeDoc = {
  id: number
  title: string
  path: string
  content: string
  tags: string
  doc_type: string
  created_at: string
  updated_at: string
}

// Paged list envelopes used by /topics and /scripts List handlers.
export type ListResponse<T> = {
  items: T[]
  total: number
  limit: number
  offset: number
}

// IP template projection returned by /ip-templates. This is a derived
// view over the knowledge_docs table, not a separate entity, so it
// carries only the list-shape quartet the UI needs.
export type IpTemplate = {
  type: string
  name: string
  description: string
  doc_count: number
}

// Auth user returned by /api/auth/login, /api/auth/register, and
// /api/auth/me. The hash is never on the wire (the Go model has
// `json:"-"`) so we don't model it here.
export type AuthUser = {
  id: number
  email: string
  name: string
  created_at: string
}

// CredentialProvider is the closed set of upstream APIs a tenant
// may store an API key for. The Go handler rejects anything else
// with a 400, so the string-literal union gives the UI an
// exhaustiveness check on switch statements that render per-
// provider badges / icons.
export type CredentialProvider =
  | 'volcengine'
  | 'anthropic'
  | 'openai'
  | 'douyin'

// CredentialProtocol is the wire shape of the upstream LLM endpoint
// a tenant may target. Phase 4 (provider-agnostic) introduced
// user-configurable Anthropic- and OpenAI-protocol endpoints on top
// of the original closed provider set; the field is optional on the
// wire because media providers (volcengine / douyin) do not carry a
// protocol in this phase.
export type CredentialProtocol = 'anthropic' | 'openai'

// Credential is the metadata projection of /api/credentials rows.
// The encrypted key NEVER appears on the wire — the Go handler
// uses `json:"-"` on EncryptedKey and a separate
// credentialResponse type that omits it, so this interface is the
// authoritative "what the UI sees" shape. Keep the field list in
// sync with handlers/credentials.go's credentialResponse.
export type Credential = {
  id: number
  user_id: number
  provider: CredentialProvider
  // Optional: protocol/base_url/model_name only apply to
  // anthropic/openai providers. The Go handler emits them with
  // `omitempty`, so a credential whose provider is volcengine
  // / douyin (or one created before Phase 4) will lack these
  // fields — the UI should treat them as not applicable.
  protocol?: CredentialProtocol
  base_url?: string
  model_name?: string
  name: string
  scope: string
  created_at: string
  updated_at: string
  // Optional: the handler omits the field when it has never been
  // stamped, so the UI should treat undefined as "never used".
  last_used_at?: string
}

// CreateCredentialInput is the body for POST /api/credentials.
// plaintext_key is required and is encrypted server-side; it is
// never echoed back in the response. protocol/base_url/model_name
// are optional on the wire but the server requires protocol +
// model_name when provider is anthropic/openai — the Add dialog
// surfaces that as conditional required fields so the wire shape
// stays valid by construction.
export interface CreateCredentialInput {
  provider: CredentialProvider
  protocol?: CredentialProtocol
  base_url?: string
  model_name?: string
  name: string
  plaintext_key: string
  scope?: string
}

// RotateCredentialInput is the body for PUT
// /api/credentials/:id/rotate. Only the secret can change on a
// rotate — name/scope edits belong on a future PATCH endpoint.
export interface RotateCredentialInput {
  plaintext_key: string
}

// CredentialListResponse mirrors the {items, total} envelope the
// List handler emits. The handler does not paginate yet; we keep
// the ListResponse<T> shape for forward-compat with limit/offset
// when the surface grows.
export type CredentialListResponse = {
  items: Credential[]
  total: number
}

// CallLogStatus is the closed set of status buckets the Go
// middleware stores in call_logs.status. The values match the
// server-side models.CallStatus* constants:
//
//   0 = ok          (2xx response)
//   1 = clientError (4xx response)
//   2 = serverError (5xx response)
//   3 = timeout     (request did not complete)
//
// We use a string-literal-numeric union (not an enum) so the
// wire format stays a plain int — the Go side stores the int
// directly and the dashboard reads it as a number. A typo on
// the UI side becomes a TypeScript error at compile time.
export type CallLogStatus = 0 | 1 | 2 | 3

// CallLog is the wire shape of GET /api/logs/:id and each
// `items` row in GET /api/logs. The fields mirror the Go
// models.CallLog struct's JSON tags 1:1. credential_id is
// nullable (an internal handler does not have one); the Go
// side uses `json:"credential_id,omitempty"`, which omits the
// field on the wire when it is nil — the TypeScript side models
// that as `number | null` so an explicit null round-trips and
// an undefined / missing field is distinguishable. error_message
// is also omitempty on the server, so the UI should treat
// `undefined` and `''` equivalently.
export interface CallLog {
  id: number
  user_id: number
  skill: string
  provider: string
  credential_id: number | null
  status: CallLogStatus
  latency_ms: number
  input_tokens: number
  output_tokens: number
  cost_cents: number
  // Count of provider-billed units that went into CostCents.
  // Stored alongside CostCents so the dashboard can
  // re-derive "what would this have cost under today's
  // pricing" without trusting the snapshot — a Phase 4
  // billing feature uses this for invoice reconciliation
  // (#46 audit: "改一次价历史 call_log 的 cost 含义全变").
  cost_units: number
  cost_currency: string
  error_message?: string
  created_at: string
}

// LogsListParams is the query string for GET /api/logs. Every
// field is optional; the server defaults to the last 7 days
// when from/to are missing, and to limit=50, offset=0 when
// pagination is missing. status is the typed int bucket
// (0/1/2/3) — the server rejects anything else with a 400, so
// the union gives the UI exhaustiveness on the filter
// dropdown.
export interface LogsListParams {
  from?: string
  to?: string
  skill?: string
  status?: CallLogStatus
  provider?: string
  search?: string
  limit?: number
  offset?: number
}

// LogsListResponse is the wire shape of GET /api/logs. items
// is always a non-null array (the server initializes to [] on
// the empty result). total is the count of rows that match the
// filter (NOT the count of rows returned in items) so the UI
// can render a "N rows" footer and wire up pagination
// controls.
export interface LogsListResponse {
  items: CallLog[]
  total: number
  limit: number
  offset: number
}

// BillingBySkillRow / BillingByProviderRow are the per-bucket
// tuples returned by GET /api/billing/summary. They mirror
// the server-side aggregation in handlers/billing.go — the
// "calls" column is COUNT(*) and "cost_cents" is
// COALESCE(SUM(cost_cents), 0). Zero-cost rows are still
// included so the operator can see "we tried skill X but
// it's free".
export interface BillingBySkillRow {
  skill: string
  calls: number
  cost_cents: number
}
export interface BillingByProviderRow {
  provider: string
  calls: number
  cost_cents: number
}

// BillingSummary is the JSON shape returned by
// GET /api/billing/summary. days_elapsed / days_in_month
// drive the linear projection (see MonthSummary). The
// projection is intentionally a flat "current_spend *
// (days_in_month / days_elapsed)" — no seasonal
// adjustment — so the operator can see "running hot"
// without smoothing hiding it.
export interface BillingSummary {
  month: string
  calls: number
  cost_cents: number
  days_elapsed: number
  days_in_month: number
  projection_cents: number
  by_skill: BillingBySkillRow[]
  by_provider: BillingByProviderRow[]
}
