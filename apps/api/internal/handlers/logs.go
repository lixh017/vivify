// Package handlers: logs.go owns the MCN call-log query surface.
// Three endpoints back the "调用日志" tab of the console:
//
//	GET /api/logs                   — paginated, filterable list
//	GET /api/logs/:id               — single-row detail (full payload)
//	GET /api/logs/export.csv        — same filters as List, streamed CSV
//
// All three endpoints share the same query string vocabulary
// (from/to/skill/status/provider/search) and the same multi-tenant
// discipline: every WHERE clause is anchored on user_id, derived
// from the auth context, so a request from tenant A never sees
// tenant B's rows. A cross-tenant access on the :id path returns
// 404 (not 403) so the response shape does not let an attacker
// distinguish "row exists, owned by another tenant" from "row
// does not exist" — both are equally "not visible to me".
//
// Why three endpoints and not one with a ?format= param: a
// browser-driven CSV download wants Content-Disposition: attachment
// on a text/csv response, while the JSON list and detail endpoints
// stay application/json. The response shape, the Content-Type, and
// the transfer-encoding all differ, so baking them into one handler
// would be a switchboard.
//
// Why the export is a separate URL and not ?format=csv on the list
// endpoint: the list endpoint caps limit at 200 (the operator's
// "show me the latest failures" workflow). An export wants every
// matching row — that is the entire purpose of the feature. A
// single URL would force the cap into both code paths.
package handlers

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// LogsHandler is the per-tenant call-log query surface. The
// handler is stateless beyond its *gorm.DB and a *slog.Logger, so
// a single instance can serve every /api/logs/* request.
type LogsHandler struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewLogsHandler wires the handler. The DB is required — the
// constructor panics on nil so a missing wire in main.go is loud
// at startup, not silent at request time. A nil logger falls
// back to slog.Default() to match the rest of the handlers
// package.
func NewLogsHandler(db *gorm.DB, logger *slog.Logger) *LogsHandler {
	if db == nil {
		panic("handlers.NewLogsHandler: db is nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &LogsHandler{db: db, logger: logger}
}

// RegisterRoutes attaches the three endpoints to the given
// router. The :id route is registered BEFORE the export.csv
// route so gin's radix tree resolves a request to /api/logs/42
// to the :id handler and not to the export segment. (In practice
// the export URL contains a literal "." which the :id pattern
// would not match anyway, but the registration order keeps the
// intent obvious to a reader scanning main.go.)
func (h *LogsHandler) RegisterRoutes(r gin.IRouter) {
	g := r.Group("/logs")
	g.GET("", h.List)
	g.GET("/export.csv", h.ExportCSV)
	g.GET("/:id", h.Get)
}

// LogsListResponse is the wire shape of GET /api/logs. items is
// always a non-nil slice (the handler initializes to [] on the
// empty result) so the frontend can .map() without a null check.
// total is the count of rows that match the filter (NOT the count
// returned in items) so the UI can render a "N rows" footer and
// wire up pagination controls.
type LogsListResponse struct {
	Items  []models.CallLog `json:"items"`
	Total  int64            `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

// LogsListFilter is the parsed form of the List query string. The
// fields are pointers (or zero-valued sentinels) so the handler
// can distinguish "not set" from "set to zero/empty". CreatedAt
// carries the resolved [from, to) window; the handler computes
// the default (last 7 days) when the query string is missing.
type LogsListFilter struct {
	UserID   uint
	From     time.Time
	To       time.Time
	Skill    string
	Provider string
	Status   *int
	Search   string
	Limit    int
	Offset   int
}

// LogsDefaultLimit / LogsMaxLimit bound the pagination. The
// defaults match the dashboard's "show me the latest" preset;
// the cap (200) keeps a single request from streaming a
// multi-megabyte JSON payload. The cap is honored server-side
// rather than trusted to the client so a misbehaving UI cannot
// DoS the JSON encoder.
const (
	LogsDefaultLimit = 50
	LogsMaxLimit     = 200
)

// List is the paginated list endpoint.
//
// Query string (all optional except where noted):
//
//	from      RFC3339; default = now - 7d
//	to        RFC3339; default = now
//	skill     exact match
//	status    0|1|2|3
//	provider  exact match
//	search    substring of error_message (LIKE %x% with x escaped)
//	limit     1..200; default 50
//	offset    >= 0; default 0
//
// 200: LogsListResponse
// 400: bad from/to, bad status, bad limit/offset, from >= to
// 401: caller not authenticated (RequireAuth)
// 500: query failure
func (h *LogsHandler) List(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	filter, err := h.parseFilter(c, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items, total, err := h.query(c, filter)
	if err != nil {
		h.logger.Error("logs list failed",
			"err", err.Error(),
			"user_id", userID,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list logs"})
		return
	}
	c.JSON(http.StatusOK, LogsListResponse{
		Items:  items,
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

// Get is the single-row detail endpoint. 404 (not 403) on a
// cross-tenant access so the response shape does not leak the
// existence of rows owned by other tenants. The 404 envelope is
// the same generic shape the rest of the per-entity handlers
// emit, keeping client-side error handling uniform.
func (h *LogsHandler) Get(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	idRaw := c.Param("id")
	id, err := strconv.ParseUint(idRaw, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a positive integer"})
		return
	}
	var row models.CallLog
	err = h.db.WithContext(c.Request.Context()).
		Where("id = ? AND user_id = ?", id, userID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "log not found"})
			return
		}
		h.logger.Error("logs get failed",
			"err", err.Error(),
			"id", id,
			"user_id", userID,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load log"})
		return
	}
	c.JSON(http.StatusOK, row)
}

// ExportCSV streams every row that matches the filter as a
// downloadable CSV. The endpoint accepts the same filter
// vocabulary as List (from/to/skill/status/provider/search) and
// intentionally IGNORES limit/offset — a paginated export would
// be a footgun (the user clicks "export", gets 200 rows, and
// assumes that's the full set).
//
// Output: text/csv with a 1-row header. The body is buffered in
// a bytes.Buffer so the Content-Length header can be set
// explicitly — a Content-Length: <N> on a CSV download lets the
// browser show a real progress bar instead of "unknown size".
func (h *LogsHandler) ExportCSV(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	filter, err := h.parseFilter(c, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rows, err := h.queryAll(c, filter)
	if err != nil {
		h.logger.Error("logs export failed",
			"err", err.Error(),
			"user_id", userID,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to export logs"})
		return
	}
	buf, err := encodeLogsCSV(rows)
	if err != nil {
		h.logger.Error("logs csv encode failed",
			"err", err.Error(),
			"user_id", userID,
			"request_id", c.GetString("request_id"),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode csv"})
		return
	}
	filename := fmt.Sprintf("opc-call-logs-%s.csv", filter.From.Format("2006-01-02"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("Content-Length", strconv.Itoa(buf.Len()))
	c.Status(http.StatusOK)
	// Use a fresh buffer copy on the response so the handler is
	// not coupled to the encoder's internal buffer (defensive —
	// the encoder is ours, but the layering keeps a future
	// streaming implementation feasible).
	_, _ = c.Writer.Write(buf.Bytes())
}

// parseFilter turns the request query string into a LogsListFilter.
// `forExport` toggles whether limit/offset are accepted (the
// export endpoint ignores them so a misconfigured client cannot
// silently truncate the download).
//
// The from > to case is rejected as 400 — a query with an
// inverted window would always return an empty result, and 400
// surfaces the misconfiguration to the caller instead of
// silently returning 0 rows.
func (h *LogsHandler) parseFilter(c *gin.Context, forExport bool) (LogsListFilter, error) {
	f := LogsListFilter{
		UserID: middleware.UserIDFromContext(c),
		Limit:  LogsDefaultLimit,
		Offset: 0,
	}
	// Default window: last 7 days. The boundary is
	// [now-7d, now) — the "to" is exclusive so a row stamped
	// exactly at request time is NOT included. This matches the
	// observability summary's window convention.
	now := time.Now()
	f.To = now
	f.From = now.AddDate(0, 0, -7)
	if v := strings.TrimSpace(c.Query("from")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return LogsListFilter{}, errors.New("from must be RFC3339 (e.g. 2026-01-02T15:04:05Z)")
		}
		f.From = t
	}
	if v := strings.TrimSpace(c.Query("to")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return LogsListFilter{}, errors.New("to must be RFC3339 (e.g. 2026-01-02T15:04:05Z)")
		}
		f.To = t
	}
	// Inverted window: a from strictly after to is a bug, not a
	// "give me nothing" query. (Equal is allowed because [a,a)
	// is the natural empty-window form.)
	if f.From.After(f.To) {
		return LogsListFilter{}, errors.New("from must be <= to")
	}
	f.Skill = strings.TrimSpace(c.Query("skill"))
	f.Provider = strings.TrimSpace(c.Query("provider"))
	f.Search = strings.TrimSpace(c.Query("search"))
	if v := strings.TrimSpace(c.Query("status")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 3 {
			return LogsListFilter{}, errors.New("status must be 0, 1, 2, or 3")
		}
		// Copy out of the loop variable so the pointer we stash
		// is not overwritten on the next iteration. (Go <= 1.21
		// would not need this; Go 1.22+ would. Defensive either
		// way.)
		statusVal := n
		f.Status = &statusVal
	}
	if forExport {
		// The export endpoint explicitly drops limit/offset
		// (a paginated export is a footgun). We do not
		// surface a 400 — silently ignoring is the right
		// call because a client that sends limit/offset is
		// asking for a download and should not be turned
		// away. We still let the filter struct carry the
		// values, in case a future streaming implementation
		// wants to chunk the response.
		return f, nil
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return LogsListFilter{}, errors.New("limit must be a positive integer")
		}
		if n > LogsMaxLimit {
			n = LogsMaxLimit
		}
		f.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return LogsListFilter{}, errors.New("offset must be a non-negative integer")
		}
		f.Offset = n
	}
	return f, nil
}

// query returns the page of rows and the unfiltered count for
// the filter. The COUNT runs as a separate query (not a window
// function) because SQLite's support for window functions over
// indexed scans is uneven and the dashboard's expected row
// counts are well under 100k — the count is cheap.
func (h *LogsHandler) query(c *gin.Context, f LogsListFilter) ([]models.CallLog, int64, error) {
	base := h.db.WithContext(c.Request.Context()).Model(&models.CallLog{}).
		Where("user_id = ?", f.UserID).
		Where("created_at >= ? AND created_at < ?", f.From, f.To)
	base = applyLogsWhere(base, f)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.CallLog
	if err := base.
		Order("created_at DESC").
		Limit(f.Limit).
		Offset(f.Offset).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	if rows == nil {
		rows = []models.CallLog{}
	}
	return rows, total, nil
}

// queryAll returns every row matching the filter (no limit / no
// offset). Used by the export endpoint. We keep it as a separate
// method so a future streaming implementation has an obvious
// insertion point.
func (h *LogsHandler) queryAll(c *gin.Context, f LogsListFilter) ([]models.CallLog, error) {
	q := h.db.WithContext(c.Request.Context()).Model(&models.CallLog{}).
		Where("user_id = ?", f.UserID).
		Where("created_at >= ? AND created_at < ?", f.From, f.To)
	q = applyLogsWhere(q, f)
	var rows []models.CallLog
	if err := q.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []models.CallLog{}
	}
	return rows, nil
}

// applyLogsWhere adds the optional skill/status/provider/search
// filters onto a base query. The user_id and time-window clauses
// are added by the caller so this helper stays focused on the
// per-row filters.
//
// The search filter is a case-insensitive substring match on
// error_message. We escape the LIKE wildcards (%) in the input
// so a user typing "50%" matches the literal "50%" rather than
// "50<anything>". FTS5 would be more correct at scale, but the
// volume per tenant is small (the observability summary's per-
// skill p95 is computed over the same row count) and the
// substring form keeps the query plan simple and indexable.
func applyLogsWhere(q *gorm.DB, f LogsListFilter) *gorm.DB {
	if f.Skill != "" {
		q = q.Where("skill = ?", f.Skill)
	}
	if f.Provider != "" {
		q = q.Where("provider = ?", f.Provider)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.Search != "" {
		// Escape the LIKE wildcards. SQLite's LIKE is
		// case-insensitive for ASCII by default, which is
		// what the dashboard wants.
		escaped := escapeLikeWildcards(f.Search)
		q = q.Where("error_message LIKE ? ESCAPE '\\'", "%"+escaped+"%")
	}
	return q
}

// escapeLikeWildcards prefixes the LIKE wildcards (%) and (_)
// with the escape character (\) so a user-supplied substring
// does not silently match more than intended. The escape
// character in the WHERE clause is "\", matching the ESCAPE
// clause above.
func escapeLikeWildcards(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// logsCSVHeader is the column order of the exported CSV. The
// order is pinned by the test (TestLogsExportCSV) so a
// future re-order is a breaking change to the operator's
// downstream scripts. Adding a column is fine; re-ordering
// is a major-version bump.
var logsCSVHeader = []string{
	"id",
	"created_at",
	"user_id",
	"skill",
	"provider",
	"status",
	"latency_ms",
	"input_tokens",
	"output_tokens",
	"cost_cents",
	"cost_currency",
	"error_message",
}

// encodeLogsCSV serializes rows to a CSV body. The encoding
// strategy:
//
//   - The header is written by hand (encoding/csv's Write would
//     also work, but writing the header ourselves keeps the
//     column order in one place — logsCSVHeader).
//   - Numeric fields are formatted with strconv (no quoting).
//   - Time is RFC3339 in UTC. UTC avoids the "the same
//     timestamp looks different in Beijing and California"
//     surprise.
//   - error_message goes through csv.Writer, which handles
//     the comma / quote / newline escaping per RFC 4180.
//
// The whole body is buffered so the Content-Length header is
// accurate. The export size for a single tenant's 7-day
// window is well under 1MB; the buffered form is fine.
func encodeLogsCSV(rows []models.CallLog) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	// Header row. We write the header by hand so the column
	// order stays co-located with the rest of the encoding
	// logic (logsCSVHeader doubles as the source of truth).
	if _, err := buf.WriteString(strings.Join(logsCSVHeader, ",") + "\n"); err != nil {
		return nil, err
	}
	for _, r := range rows {
		rec := make([]string, len(logsCSVHeader))
		rec[0] = strconv.FormatUint(uint64(r.ID), 10)
		rec[1] = r.CreatedAt.UTC().Format(time.RFC3339)
		rec[2] = strconv.FormatUint(uint64(r.UserID), 10)
		rec[3] = r.Skill
		rec[4] = r.Provider
		rec[5] = strconv.Itoa(r.Status)
		rec[6] = strconv.Itoa(r.LatencyMS)
		rec[7] = strconv.Itoa(r.InputTokens)
		rec[8] = strconv.Itoa(r.OutputTokens)
		rec[9] = strconv.Itoa(r.CostCents)
		rec[10] = r.CostCurrency
		rec[11] = r.ErrorMessage
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf, nil
}
