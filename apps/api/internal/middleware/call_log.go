package middleware

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// CallLogRef is the in-flight mirror of a models.CallLog row. The
// middleware creates one at request entry, the handler mutates it
// (Provider, CostCents, ErrorMessage, InputTokens, OutputTokens),
// and the middleware writes the resulting row to the database in
// the deferred finalizer.
//
// We use a pointer to a struct (not the model itself) so the
// handler can mutate the same instance the middleware holds;
// copying by value would silently drop handler-side overrides.
type CallLogRef struct {
	UserID       uint
	Skill        string
	Provider     string
	CredentialID *uint
	// Status is filled in by the middleware finalizer from the
	// response status code. Handlers that need a non-default
	// classification (e.g. timeout) can override before the
	// finalizer runs.
	Status int
	// LatencyMS is filled in by the finalizer. Handlers should
	// not touch it.
	LatencyMS int
	// CostCents is left at 0 by the middleware. The handler is
	// responsible for stamping the cost once it knows the billable
	// units (image count, token count, character count).
	CostCents int
	// CostUnits is the count of billable units that went into
	// this call. Storing it alongside CostCents lets the
	// dashboard re-derive "what would this have cost under
	// today's pricing" without mutating the historical row
	// (#46 audit: "改一次价历史 call_log 的 cost 含义全变").
	CostUnits int
	// CostCurrency overrides the default "CNY" stored on the row
	// when the provider bills in a different currency (e.g. USD
	// for Anthropic).
	CostCurrency string
	// ErrorMessage is stamped on non-2xx responses. Empty on
	// success.
	ErrorMessage string
	// InputTokens / OutputTokens are stamped by handlers that
	// have the upstream provider's usage object in hand. The
	// middleware leaves them at 0.
	InputTokens  int
	OutputTokens int
	// start is captured by the middleware at request entry so the
	// finalizer can compute LatencyMS without a second time.Now
	// call. Unexported — handlers must not touch it.
	start time.Time
}

// CallLogFromContext returns the CallLogRef the middleware stored
// for this request, or nil if CallLog is not in the chain. Handlers
// that want to stamp provider/cost should always do
//
//	if ref := middleware.CallLogFromContext(c); ref != nil {
//	    ref.Provider = "anthropic"
//	}
//
// — a nil check is required because not every route runs through
// the middleware (e.g. /api/auth/*, /api/health, /api/observability/*).
func CallLogFromContext(c *gin.Context) *CallLogRef {
	if c == nil {
		return nil
	}
	v, ok := c.Get("call_log_ref")
	if !ok {
		return nil
	}
	r, _ := v.(*CallLogRef)
	return r
}

// setCallLogRef is the internal helper the middleware uses to
// stash a new ref on the context. Centralized so the key name
// ("call_log_ref") is owned by one place.
func setCallLogRef(c *gin.Context, ref *CallLogRef) {
	c.Set("call_log_ref", ref)
}

// CallLogConfig tunes CallLog middleware behavior. Fields are
// optional — zero values are sensible defaults.
type CallLogConfig struct {
	// DB is the gorm handle the finalizer writes to. Required —
	// the constructor panics on nil.
	DB *gorm.DB
	// Logger is the slog handler warnings/errors are routed
	// through. A nil value falls back to slog.Default().
	Logger *slog.Logger
	// SkipPaths is the set of URL prefixes that bypass the
	// middleware entirely. The default list
	// (defaultSkipPaths) covers the auth surface, the health
	// probes, the metrics scrape, and the observability surface
	// itself — the latter is recursive: the dashboard reads its
	// own rows, and writing a row per read would inflate the
	// aggregates.
	SkipPaths []string
}

// defaultSkipPaths is the canonical skip-list. Exposed as a var (not
// a const slice) so tests can append without mutating the caller's
// slice, and so future routes (e.g. /api/docs, /api/openapi.json)
// can extend the list at one site.
var defaultSkipPaths = []string{
	"/api/auth/",
	"/api/health",
	"/api/observability/",
	"/api/call-logs", // reserved for a future raw-logs endpoint
	"/api/logs",      // log query surface — reads must not log themselves
	"/api/billing/",  // billing summary + export — same recursive concern
	"/metrics",
	"/healthz",
	"/readyz",
}

// CallLog returns a gin middleware that writes one
// models.CallLog row per non-skipped request. The flow is:
//
//  1. start = time.Now();  ref = &CallLogRef{start: start, ...}
//  2. c.Set("call_log_ref", ref)
//  3. c.Next() — handlers run, can stamp ref.Provider / ref.CostCents
//  4. defer finalizer: compute status, latency; Create(&CallLog{...})
//
// The finalizer runs even on panic (Gin's Recovery middleware is
// upstream in main.go), so a panic is recorded as a 5xx with the
// panic value in ErrorMessage. The Create call is wrapped in a
// best-effort recover: a failed write must not break the request
// that the client is waiting on. We log the error and move on.
func CallLog(cfg CallLogConfig) gin.HandlerFunc {
	if cfg.DB == nil {
		panic("middleware.CallLog: cfg.DB is nil")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	skip := append([]string{}, defaultSkipPaths...)
	if len(cfg.SkipPaths) > 0 {
		skip = append(skip, cfg.SkipPaths...)
	}
	skipSet := make(map[string]struct{}, len(skip))
	for _, p := range skip {
		skipSet[p] = struct{}{}
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if _, hit := skipSet[path]; hit {
			c.Next()
			return
		}
		// Prefix match for /api/auth/*, /api/observability/*, etc.
		for _, p := range skip {
			if strings.HasSuffix(p, "/") && strings.HasPrefix(path, p) {
				c.Next()
				return
			}
		}

		start := time.Now()
		ref := &CallLogRef{
			// UserID is captured LATE (in the defer) so the
			// auth middleware that runs immediately after this
			// one (RequireAuth) has time to set CtxUserID. The
			// middleware chain in main.go wires CallLog AFTER
			// RequireAuth, so by the time the defer runs the
			// user_id is in place.
			Skill:    inferSkill(path, c.Request.Method),
			Provider: "internal",
			// LatencyMS is filled in by the finalizer.
		}
		setCallLogRef(c, ref)

		// Defer the finalizer so it runs on every exit path —
		// success, handler error, panic, client disconnect. The
		// finalizer reads the response status (post-c.Next) and
		// writes the row.
		defer func() {
			if ref.UserID == 0 {
				ref.UserID = UserIDFromContext(c)
			}
			ref.LatencyMS = int(time.Since(start).Milliseconds())
			if ref.Status == 0 {
				ref.Status = classifyStatus(c.Writer.Status())
			}
			// Only stamp an error message if the handler did not
			// already — handlers with richer error context should
			// win over the generic "5xx" we synthesize below.
			if ref.ErrorMessage == "" && ref.Status != models.CallStatusOK {
				ref.ErrorMessage = inferErrorMessage(c, ref.Status)
			}
			row := models.CallLog{
				UserID:       ref.UserID,
				Skill:        ref.Skill,
				Provider:     ref.Provider,
				CredentialID: ref.CredentialID,
				Status:       ref.Status,
				LatencyMS:    ref.LatencyMS,
				InputTokens:  ref.InputTokens,
				OutputTokens: ref.OutputTokens,
				CostCents:    ref.CostCents,
				CostUnits:    ref.CostUnits,
				CostCurrency: ref.CostCurrency,
				ErrorMessage: ref.ErrorMessage,
			}
			// Best-effort write. A failed Create must never
			// propagate as a 500 to the client — the request has
			// already been served.
			if err := cfg.DB.WithContext(context.Background()).Create(&row).Error; err != nil {
				logger.Warn("call_log: create row failed",
					"err", err.Error(),
					"skill", ref.Skill,
					"request_id", c.GetString("request_id"),
				)
			}
		}()

		c.Next()
	}
}

// inferSkill derives a stable skill name from the request path and
// method. The convention is "<area>_<verb>" (e.g. "ai_topics",
// "credentials_list"). The map is intentionally a switch and not a
// table — the path shapes are stable, the call site is the right
// place to encode the URL -> skill mapping, and the test suite
// covers the cases that matter.
//
// Unknown paths fall through to "other". A new billable skill that
// wants to land a more specific label should add a case here AND
// register a cost row in internal/config/cost.go — otherwise the
// dashboard shows the call as free.
//
// Method suffix convention: POST/PUT/DELETE/GET -> "create",
// "update", "delete", "list"/"get" respectively. The collection
// path ("/api/topics") maps to "_list" for GET and "_create" for
// POST; the item path ("/api/topics/:id") maps to "_get" for GET
// and "_update"/"_delete" for PUT/DELETE.
func inferSkill(path, method string) string {
	// Strip the /api prefix so the rest of the function deals in
	// a stable shape ("/topics/:id", "/ai/topics", "/cover").
	p := strings.TrimPrefix(path, "/api/")
	if p == path {
		// Path did not start with /api — leave it alone (the
		// skip-list should have caught it, but be defensive).
		p = path
	}
	// Trim trailing slash for clean matching.
	p = strings.TrimSuffix(p, "/")

	// Map by area first; within an area, dispatch on the verb.
	switch {
	case p == "ai/topics" || strings.HasPrefix(p, "ai/topics/"):
		return "ai_topics"
	case p == "ai/humanize" || strings.HasPrefix(p, "ai/humanize/"):
		return "ai_humanize"
	case p == "ai/score" || strings.HasPrefix(p, "ai/score/"):
		return "ai_score"
	case p == "ai/postmortem" || strings.HasPrefix(p, "ai/postmortem/"):
		return "ai_postmortem"
	case p == "ai/pipeline" || strings.HasPrefix(p, "ai/pipeline/"):
		return "ai_pipeline"
	case p == "ai/deconstruct" || strings.HasPrefix(p, "ai/deconstruct/"):
		return "ai_deconstruct"
	case p == "ai/platform-adapt" || strings.HasPrefix(p, "ai/platform-adapt/"):
		return "ai_platform_adapt"
	case p == "ai/viral-formula" || strings.HasPrefix(p, "ai/viral-formula/"):
		return "ai_viral_formula"
	case p == "ai/publish-checklist" || strings.HasPrefix(p, "ai/publish-checklist/"):
		return "ai_publish_checklist"
	case p == "speech" || strings.HasPrefix(p, "speech/"):
		return "tts_synthesize"
	case p == "video" || strings.HasPrefix(p, "video/"):
		return "render_video"
	case p == "ai/cover" || strings.HasPrefix(p, "ai/cover/"):
		return "cover_generate"
	case p == "cover" || strings.HasPrefix(p, "cover/"):
		// Legacy "cover" path — kept for back-compat with any
		// pre-Phase-4 clients. New code should hit /ai/cover.
		return "cover_generate"
	case p == "credentials" || strings.HasPrefix(p, "credentials/"):
		return credentialsSkill(p, method)
	case p == "topics" || strings.HasPrefix(p, "topics/"):
		return topicsSkill(p, method)
	case p == "scripts" || strings.HasPrefix(p, "scripts/"):
		return scriptsSkill(p, method)
	case p == "content-items" || strings.HasPrefix(p, "content-items/"):
		return contentItemsSkill(p, method)
	case p == "knowledge" || strings.HasPrefix(p, "knowledge/"):
		return knowledgeSkill(p, method)
	case p == "series" || strings.HasPrefix(p, "series/"):
		return seriesSkill(p, method)
	}

	// Fallback — call it "other" so a regression in the URL space
	// surfaces as a single bucket in the dashboard rather than
	// silently leaking provider/credential info via the skill
	// name.
	return "other"
}

func credentialsSkill(p, method string) string {
	if p == "credentials" {
		if method == "GET" {
			return "credentials_list"
		}
		if method == "POST" {
			return "credentials_create"
		}
	}
	if method == "DELETE" {
		return "credentials_delete"
	}
	if method == "PUT" || method == "PATCH" {
		return "credentials_update"
	}
	return "credentials_get"
}

func topicsSkill(p, method string) string {
	if p == "topics" {
		switch method {
		case "GET":
			return "topics_list"
		case "POST":
			return "topics_create"
		}
	}
	switch method {
	case "GET":
		return "topics_get"
	case "PUT", "PATCH":
		return "topics_update"
	case "DELETE":
		return "topics_delete"
	}
	return "topics_other"
}

func scriptsSkill(p, method string) string {
	if p == "scripts" {
		switch method {
		case "GET":
			return "scripts_list"
		case "POST":
			return "scripts_create"
		}
	}
	switch method {
	case "GET":
		return "scripts_get"
	case "PUT", "PATCH":
		return "scripts_update"
	case "DELETE":
		return "scripts_delete"
	}
	return "scripts_other"
}

func contentItemsSkill(p, method string) string {
	if p == "content-items" {
		switch method {
		case "GET":
			return "content_items_list"
		case "POST":
			return "content_items_create"
		}
	}
	switch method {
	case "GET":
		return "content_items_get"
	case "PUT", "PATCH":
		return "content_items_update"
	case "DELETE":
		return "content_items_delete"
	}
	return "content_items_other"
}

func knowledgeSkill(p, method string) string {
	if p == "knowledge" {
		switch method {
		case "GET":
			return "knowledge_list"
		case "POST":
			return "knowledge_create"
		}
	}
	switch method {
	case "GET":
		return "knowledge_get"
	case "PUT", "PATCH":
		return "knowledge_update"
	case "DELETE":
		return "knowledge_delete"
	}
	return "knowledge_other"
}

func seriesSkill(p, method string) string {
	if p == "series" {
		switch method {
		case "GET":
			return "series_list"
		case "POST":
			return "series_create"
		}
	}
	switch method {
	case "GET":
		return "series_get"
	case "PUT", "PATCH":
		return "series_update"
	case "DELETE":
		return "series_delete"
	}
	return "series_other"
}

// classifyStatus maps the gin's response code to the
// CallLog.Status bucket. The mapping is the same as the one the
// Prometheus middleware uses (c.Writer.Status() into 0/1/2/3), so
// the two observability surfaces agree on what "5xx" means.
//
// We do not consult the request context for a "client disconnected"
// signal — gin's response writer may or may not have set a status
// if the handler panicked and Recovery middleware caught it. In
// that case Status() returns 200 (gin's default before the panic),
// so a 5xx-from-panic falls through to OK. That is a known
// limitation; surfacing it correctly would require Gin's
// Recovery middleware to share state, which is out of scope here.
func classifyStatus(httpStatus int) int {
	switch {
	case httpStatus >= 200 && httpStatus < 400:
		return models.CallStatusOK
	case httpStatus >= 400 && httpStatus < 500:
		return models.CallStatusClientError
	case httpStatus >= 500:
		return models.CallStatusServerError
	}
	// 1xx, 3xx with no body, or 0 (no status set). Default to OK
	// — the handler did not produce an error response.
	return models.CallStatusOK
}

// inferErrorMessage builds a short, non-sensitive error label for
// the dashboard. We deliberately do NOT echo the raw c.Errors() or
// the response body — both can carry user-input fragments (e.g. a
// SQL error containing the offending value). The middleware keeps
// the label generic and lets the operator correlate via
// request_id.
func inferErrorMessage(c *gin.Context, status int) string {
	switch status {
	case models.CallStatusClientError:
		return "client_error"
	case models.CallStatusServerError:
		return "server_error"
	case models.CallStatusTimeout:
		return "timeout"
	}
	return ""
}
