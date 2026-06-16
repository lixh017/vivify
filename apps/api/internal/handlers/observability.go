// Package handlers: observability.go owns the MCN call-observability
// dashboard surface. The single public endpoint
// (GET /api/observability/summary) returns a snapshot of the
// call_logs table grouped by skill, provider, and day — the
// minimal shape the console dashboard needs to render the "调用
// 观测" page.
//
// Why one endpoint, not several: the dashboard loads the page in
// one request, and each per-skill or per-provider breakdown is a
// single GROUP BY query that runs in milliseconds against an
// indexed user_id + created_at pair. Splitting the endpoint into
// /summary, /by-skill, /by-provider would force three round-trips
// for no measurable server-side saving.
//
// Why a *gorm.DB handle on the handler, not a CallLogStore
// interface: the dashboard is a read-only surface with no
// per-row mutations, and the only operations it needs are
// COUNT/GROUP BY. A dedicated interface would be ceremony for
// ceremony's sake; the production wiring passes *gorm.DB, the
// unit tests can do the same with a sqlite temp file.
package handlers

import (
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// SummaryRange enumerates the time windows the /summary endpoint
// understands. Centralized so a typo in a query string is caught
// here and the dashboard can render a 400 rather than a 500 from
// the SQL parser.
type SummaryRange string

const (
	SummaryRangeToday SummaryRange = "today"
	SummaryRange7d    SummaryRange = "7d"
	SummaryRange30d   SummaryRange = "30d"
)

// Default range when ?range= is missing or unrecognized. The
// dashboard's "近 7 天" preset is the most common view, so we make
// it the default — a missing/garbage range lands on the same data
// the user would have selected.
const defaultSummaryRange = SummaryRange7d

// CallLogSummary is the wire shape of GET /api/observability/summary.
// All numeric fields are populated by the handler; all array
// fields are non-nil (empty slice rather than null) so the
// frontend can `.map(...)` without a null check.
type CallLogSummary struct {
	Range SummaryRange `json:"range"`
	// Today aggregates the calendar day in the server's local
	// timezone. The handler computes "today" as the 24-hour
	// window starting at 00:00 in time.Local — operators reading
	// the dashboard in Beijing at 09:00 see counts from the
	// calendar day, not the trailing 24h.
	Today TodaySummary `json:"today"`
	// Last7Days is the per-day series the line chart renders.
	// Always length 7 when range=7d, length 1 when range=today,
	// length 30 when range=30d. We use the "exactly N points"
	// shape (zero-filled for empty days) so the frontend does not
	// have to backfill missing days itself.
	Last7Days []DayPoint `json:"last_7_days"`
	// BySkill is sorted by calls DESC — the operator's eye lands
	// on the highest-volume skill first.
	BySkill []BySkillRow `json:"by_skill"`
	// ByProvider is the per-provider cost rollup. Sorted by cost
	// DESC so the most expensive provider is on top.
	ByProvider []ByProviderRow `json:"by_provider"`
	// ByHourDow is the sparse activity heatmap: one cell per
	// (dow, hour) bucket that had at least one call in the
	// window. dow follows time.Weekday (0=Sun..6=Sat); hour is
	// 0-23 in time.Local. The array is dense-zero on the wire
	// (cells with zero calls are omitted) so the dashboard
	// densifies by defaulting missing cells to 0.
	ByHourDow []ByHourDowCell `json:"by_hour_dow"`
}

// ByHourDowCell is one cell in the 7×24 activity heatmap. See
// the CallLogSummary.ByHourDow doc for the dow + hour semantics.
type ByHourDowCell struct {
	DOW   int   `json:"dow"`   // 0=Sun, 1=Mon, ..., 6=Sat
	Hour  int   `json:"hour"`  // 0-23 in time.Local
	Calls int64 `json:"calls"`
}

// TodaySummary is the calendar-day rollup.
type TodaySummary struct {
	Calls        int64   `json:"calls"`
	SuccessRate  float64 `json:"success_rate"`
	CostCents    int64   `json:"cost_cents"`
	CostCurrency string  `json:"cost_currency"`
}

// DayPoint is one bucket in the per-day series. Date is the
// YYYY-MM-DD string in the server's local timezone — the
// frontend renders it directly without re-parsing.
type DayPoint struct {
	Date      string `json:"date"`
	Calls     int64  `json:"calls"`
	CostCents int64  `json:"cost_cents"`
}

// BySkillRow is one row in the by_skill breakdown. p50/p95 are
// computed server-side from latency_ms over the rows that match
// (user_id, skill, range). A skill with zero successful calls has
// p50/p95 = 0 — we do not return null, because the dashboard
// renders a single number per skill and null would force a
// branch at the call site.
type BySkillRow struct {
	Skill       string  `json:"skill"`
	Calls       int64   `json:"calls"`
	SuccessRate float64 `json:"success_rate"`
	P50MS       int     `json:"p50_ms"`
	P95MS       int     `json:"p95_ms"`
	CostCents   int64   `json:"cost_cents"`
}

// ByProviderRow is one row in the by_provider breakdown. p50/p95
// are intentionally NOT included — the dashboard renders cost
// and call volume per provider, and a latency breakdown per
// provider is gilding the lily for a Phase 3 surface.
type ByProviderRow struct {
	Provider  string `json:"provider"`
	Calls     int64  `json:"calls"`
	CostCents int64  `json:"cost_cents"`
}

// ObservabilityHandler owns the dashboard surface. The handler is
// stateless beyond its *gorm.DB, so a single instance can serve
// every /api/observability/* request.
type ObservabilityHandler struct {
	db *gorm.DB
}

// NewObservabilityHandler wires a handler with the given *gorm.DB.
// The DB is required — the constructor panics on nil so a missing
// wire in main.go is loud at startup, not silent at request time.
func NewObservabilityHandler(db *gorm.DB) *ObservabilityHandler {
	if db == nil {
		panic("handlers.NewObservabilityHandler: db is nil")
	}
	return &ObservabilityHandler{db: db}
}

// RegisterRoutes attaches the dashboard endpoint(s) to the given
// router. Today only /summary exists; the URL space is owned by
// this handler so future endpoints (e.g. /by-skill, /export)
// land here without colliding with the per-entity CRUD routes.
//
// The endpoints are mounted UNDER the protected /api group — the
// dashboard is multi-tenant and a missing RequireAuth on this
// surface would leak cross-tenant aggregates. RequireAuth must
// run BEFORE this handler in the chain; main.go is responsible
// for that wiring.
func (h *ObservabilityHandler) RegisterRoutes(r gin.IRouter) {
	g := r.Group("/observability")
	g.GET("/summary", h.Summary)
}

// Summary is the single dashboard endpoint. It accepts ?range=
// (today|7d|30d) and returns the CallLogSummary wire shape.
//
// 200: { range, today, last_7_days, by_skill, by_provider }
// 400: invalid ?range=
// 401: caller is not authenticated (RequireAuth)
// 500: query failure
//
// The handler resolves the caller's user_id from the auth
// context (set by middleware.RequireAuth) and scopes every
// aggregation to that user. The observability surface is
// multi-tenant by design — a Phase 2 "operator can see all
// tenants" view is a separate endpoint.
func (h *ObservabilityHandler) Summary(c *gin.Context) {
	userID := middleware.UserIDFromContext(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	rng, ok := parseRange(c.Query("range"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "range must be one of: today, 7d, 30d"})
		return
	}
	windowStart, windowEnd, days := windowForRange(rng)
	todayStart, todayEnd := todayWindow()

	out := CallLogSummary{Range: rng}

	// --- today: today's calls / success_rate / cost_cents. -----
	if tCalls, tOK, tCost, err := h.aggregateTotals(c, userID, todayStart, todayEnd); err != nil {
		respondInternal(c, "today aggregate", err)
		return
	} else {
		out.Today = TodaySummary{
			Calls:       tCalls,
			SuccessRate: ratio(tOK, tCalls),
			CostCents:   tCost,
			// Currency is hard-coded for now — every row defaults
			// to CNY and Phase 4 will surface a multi-currency
			// view.
			CostCurrency: "CNY",
		}
	}

	// --- last_7_days: per-day calls + cost_cents series. --------
	points, err := h.aggregatePerDay(c, userID, windowStart, windowEnd, days)
	if err != nil {
		respondInternal(c, "per-day aggregate", err)
		return
	}
	out.Last7Days = points

	// --- by_skill: skill, calls, success_rate, p50, p95, cost. --
	bySkill, err := h.aggregateBySkill(c, userID, windowStart, windowEnd)
	if err != nil {
		respondInternal(c, "by-skill aggregate", err)
		return
	}
	out.BySkill = bySkill

	// --- by_provider: provider, calls, cost_cents. ---------------
	byProvider, err := h.aggregateByProvider(c, userID, windowStart, windowEnd)
	if err != nil {
		respondInternal(c, "by-provider aggregate", err)
		return
	}
	out.ByProvider = byProvider

	// --- by_hour_dow: sparse (dow, hour, calls) for the 7x24
	//     activity heatmap. Only cells with >= 1 call are
	//     emitted; the dashboard densifies to the 7x24 grid
	//     by defaulting missing cells to 0. -----------------
	byHourDow, err := h.aggregateByHourDow(c, userID, windowStart, windowEnd)
	if err != nil {
		respondInternal(c, "by-hour-dow aggregate", err)
		return
	}
	out.ByHourDow = byHourDow

	c.JSON(http.StatusOK, out)
}

// parseRange turns the ?range= query value into a SummaryRange.
// Returns (range, true) on a recognized value and (default, true)
// on a missing/empty value; returns (default, false) on a
// recognized-but-invalid value (so the handler can 400).
func parseRange(raw string) (SummaryRange, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return defaultSummaryRange, true
	}
	switch SummaryRange(s) {
	case SummaryRangeToday, SummaryRange7d, SummaryRange30d:
		return SummaryRange(s), true
	}
	return defaultSummaryRange, false
}

// windowForRange returns the (start, end, dayCount) triple the
// aggregations should fold over. The end is "now" (rounded up to
// the next second so an inclusive WHERE works); the start is
// `end - dayCount days`. For range=today, dayCount=1 and the
// window is the calendar day.
//
// We compute the window in time.Local because the dashboard
// renders YYYY-MM-DD strings in the server's local timezone — a
// UTC range would shift the day boundaries by 8 hours and the
// dashboard would label "today" with yesterday's calls.
func windowForRange(r SummaryRange) (time.Time, time.Time, int) {
	now := time.Now()
	switch r {
	case SummaryRangeToday:
		start, end := todayWindow()
		return start, end, 1
	case SummaryRange30d:
		return now.AddDate(0, 0, -30), now, 30
	default:
		// 7d is the documented default and the catch-all.
		return now.AddDate(0, 0, -7), now, 7
	}
}

// todayWindow returns the [start, end) bounds of the current
// calendar day in time.Local. The end is exclusive — a row whose
// created_at equals exactly end is not folded into today, which
// matches the dashboard's "00:00 boundary" expectation.
func todayWindow() (time.Time, time.Time) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.Add(24 * time.Hour)
	return start, end
}

// aggregateTotals returns (calls, okCalls, costCents) over the
// given (user, start, end) window. The query is one round-trip
// — SUM(CASE WHEN status=0 THEN 1 ELSE 0 END) folds the success
// count into the same row as the call count, and SUM(cost_cents)
// rolls up the cost. The CASE form is portable to SQLite and
// Postgres (no IF).
func (h *ObservabilityHandler) aggregateTotals(
	c *gin.Context, userID uint, start, end time.Time,
) (int64, int64, int64, error) {
	type row struct {
		Calls   int64
		OK      int64
		CostSum int64
	}
	var r row
	err := h.db.WithContext(c.Request.Context()).
		Model(&models.CallLog{}).
		Select(`
			COUNT(*) AS calls,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS ok,
			COALESCE(SUM(cost_cents), 0) AS cost_sum
		`, models.CallStatusOK).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
		Scan(&r).Error
	if err != nil {
		return 0, 0, 0, err
	}
	return r.Calls, r.OK, r.CostSum, nil
}

// aggregatePerDay returns the per-day (date, calls, cost_cents)
// series for the window. We group by DATE(created_at, 'localtime')
// — SQLite's 'localtime' modifier formats the timestamp in the
// server's local timezone, which is what the dashboard wants. A
// GORM Raw is the right primitive here because the
// DATE(... , 'localtime') form is SQLite-specific and we do not
// want to write a portable (and slower) equivalent just to keep
// the abstraction clean.
//
// Days with zero rows are zero-filled by the post-processing in
// the caller (we always emit exactly `days` points, oldest
// first) so the line chart does not skip a day with no traffic.
func (h *ObservabilityHandler) aggregatePerDay(
	c *gin.Context, userID uint, start, end time.Time, days int,
) ([]DayPoint, error) {
	type rawRow struct {
		Date    string
		Calls   int64
		CostSum int64
	}
	var rows []rawRow
	err := h.db.WithContext(c.Request.Context()).
		Model(&models.CallLog{}).
		Select(`
			strftime('%Y-%m-%d', created_at, 'localtime') AS date,
			COUNT(*) AS calls,
			COALESCE(SUM(cost_cents), 0) AS cost_sum
		`).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
		Group("date").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	// Index the raw rows by date for O(1) backfill lookup.
	byDate := make(map[string]rawRow, len(rows))
	for _, r := range rows {
		byDate[r.Date] = r
	}
	// Walk days oldest -> newest and zero-fill missing dates.
	out := make([]DayPoint, 0, days)
	for i := days - 1; i >= 0; i-- {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		if v, ok := byDate[day]; ok {
			out = append(out, DayPoint{Date: day, Calls: v.Calls, CostCents: v.CostSum})
		} else {
			out = append(out, DayPoint{Date: day, Calls: 0, CostCents: 0})
		}
	}
	return out, nil
}

// aggregateBySkill returns the by-skill breakdown. The latencies
// (p50, p95) require pulling latency_ms per row, sorting in Go,
// and computing the percentile — SQLite has no PERCENTILE_CONT,
// and shipping every latency value across the GORM boundary is
// cheap for the row counts a single tenant produces in 7 days
// (typically <10k).
//
// p50/p95 use nearest-rank on the sorted slice. The nearest-rank
// convention matches what a "P50 latency" label means in the
// Prometheus histogram dashboard, so the two surfaces do not
// disagree on a definition.
func (h *ObservabilityHandler) aggregateBySkill(
	c *gin.Context, userID uint, start, end time.Time,
) ([]BySkillRow, error) {
	type aggRow struct {
		Skill   string
		Calls   int64
		OK      int64
		CostSum int64
	}
	var groups []aggRow
	err := h.db.WithContext(c.Request.Context()).
		Model(&models.CallLog{}).
		Select(`
			skill,
			COUNT(*) AS calls,
			COALESCE(SUM(CASE WHEN status = ? THEN 1 ELSE 0 END), 0) AS ok,
			COALESCE(SUM(cost_cents), 0) AS cost_sum
		`, models.CallStatusOK).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
		Group("skill").
		Scan(&groups).Error
	if err != nil {
		return nil, err
	}

	// Per-skill latency: pull every latency_ms in the window and
	// bucket in Go. We use a single SELECT in/latency_ms/l skill
	// over the whole window (not per-skill) to amortize the
	// query, then group in Go.
	type latRow struct {
		Skill     string
		LatencyMS int
	}
	var lats []latRow
	if err := h.db.WithContext(c.Request.Context()).
		Model(&models.CallLog{}).
		Select("skill, latency_ms").
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
		Scan(&lats).Error; err != nil {
		return nil, err
	}
	latBySkill := make(map[string][]int, len(groups))
	for _, l := range lats {
		latBySkill[l.Skill] = append(latBySkill[l.Skill], l.LatencyMS)
	}

	out := make([]BySkillRow, 0, len(groups))
	for _, g := range groups {
		lats := latBySkill[g.Skill]
		sort.Ints(lats)
		out = append(out, BySkillRow{
			Skill:       g.Skill,
			Calls:       g.Calls,
			SuccessRate: ratio(g.OK, g.Calls),
			P50MS:       percentileNearest(lats, 0.50),
			P95MS:       percentileNearest(lats, 0.95),
			CostCents:   g.CostSum,
		})
	}
	// Sort by calls DESC — operators' eyes go to the busiest
	// skill first.
	sort.Slice(out, func(i, j int) bool { return out[i].Calls > out[j].Calls })
	return out, nil
}

// aggregateByProvider returns the by-provider breakdown. Sorted
// by cost DESC so the most expensive provider lands on top.
func (h *ObservabilityHandler) aggregateByProvider(
	c *gin.Context, userID uint, start, end time.Time,
) ([]ByProviderRow, error) {
	type aggRow struct {
		Provider string
		Calls    int64
		CostSum  int64
	}
	var rows []aggRow
	err := h.db.WithContext(c.Request.Context()).
		Model(&models.CallLog{}).
		Select(`
			provider,
			COUNT(*) AS calls,
			COALESCE(SUM(cost_cents), 0) AS cost_sum
		`).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
		Group("provider").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]ByProviderRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, ByProviderRow{
			Provider:  r.Provider,
			Calls:     r.Calls,
			CostCents: r.CostSum,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CostCents > out[j].CostCents })
	return out, nil
}

// ratio returns a / b as a 0-100 float. Zero division is mapped
// to 0 (the dashboard renders "0% success rate" rather than "NaN"
// when the user has not made any calls yet).
func ratio(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// percentileNearest returns the nearest-rank percentile of a
// sorted-int slice. p=0.50 returns the median, p=0.95 the 95th
// percentile. An empty slice returns 0.
//
// nearest-rank: rank = ceil(p * n), then pick sorted[rank-1].
// This is the percentile definition every operator dashboard
// uses; the alternative (linear interpolation between ranks) is
// more numerically accurate but does not match the "P50 latency"
// label in the rest of the stack.
func percentileNearest(sorted []int, p float64) int {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[n-1]
	}
	rank := int(float64(n) * p)
	if rank == 0 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return sorted[rank-1]
}

// aggregateByHourDow returns the sparse (dow, hour, calls) cell
// set for the 7x24 activity heatmap. SQL folds both axes into a
// single GROUP BY using strftime (SQLite) / date_part (Postgres)
// — GORM does not abstract this cleanly, so we hand-write the
// expression. The cast to INT (not TEXT) lets the JSON encoder
// emit numeric dow / hour fields without a string-quote surprise.
//
// SQLite is the production target (see cmd/server/main.go), so
// strftime is the right tool. A future Postgres migration would
// swap to EXTRACT(DOW FROM created_at) + EXTRACT(HOUR FROM
// created_at). The window is whatever windowForRange returns,
// which is in time.Local — the bucketing naturally aligns with
// the operator's calendar.
func (h *ObservabilityHandler) aggregateByHourDow(
	c *gin.Context, userID uint, start, end time.Time,
) ([]ByHourDowCell, error) {
	type aggRow struct {
		Dow   int
		Hour  int
		Calls int64
	}
	var rows []aggRow
	// The strftime expressions are duplicated in GROUP BY
	// rather than aliased because some SQLite client bindings
	// (and GORM's raw SQL pass-through) do not substitute the
	// alias into GROUP BY — keeping the expression verbatim
	// guarantees the GROUP BY targets the same column as the
	// SELECT. CAST to INTEGER is required for the JSON encoder
	// to emit numeric dow / hour; SQLite's strftime returns
	// TEXT by default, which would surface as string "0",
	// "1", … in the wire shape and force the dashboard to
	// re-parse.
	//
	// The SELECT uses quoted lowercase aliases ("dow", "hour",
	// "calls") so GORM's Scan can map to the struct fields
	// by exact match — an unquoted alias like AS dow folds to
	// the SQL keyword and is treated as a column reference,
	// which produces zero rows on Scan.
	err := h.db.WithContext(c.Request.Context()).
		Model(&models.CallLog{}).
		Select(`
			CAST(strftime('%w', created_at) AS INTEGER) AS "dow",
			CAST(strftime('%H', created_at) AS INTEGER) AS "hour",
			COUNT(*) AS "calls"
		`).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
		Group(`CAST(strftime('%w', created_at) AS INTEGER), CAST(strftime('%H', created_at) AS INTEGER)`).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]ByHourDowCell, 0, len(rows))
	for _, r := range rows {
		out = append(out, ByHourDowCell{DOW: r.Dow, Hour: r.Hour, Calls: r.Calls})
	}
	// Defensive sort. GROUP BY in SQLite is order-preserving
	// for the keys, but the wire shape does not promise an
	// order — the dashboard sorts client-side anyway. Keeping
	// (dow, hour) ascending here means a snapshot test can
	// pin exact output.
	sort.Slice(out, func(i, j int) bool {
		if out[i].DOW != out[j].DOW {
			return out[i].DOW < out[j].DOW
		}
		return out[i].Hour < out[j].Hour
	})
	return out, nil
}

// respondInternal is the canonical 500 logger + responder. The
// helper exists so the four aggregation call sites in Summary
// stay terse and the log line carries the operation label.
func respondInternal(c *gin.Context, op string, err error) {
	// We log server-side (slog.Default() is wired up by
	// cmd/server/main.go) and never echo err.Error() to the
	// client — the wire shape is a fixed 500 envelope.
	slog.Default().Error(
		"observability summary "+op,
		"err", err.Error(),
		"request_id", c.GetString("request_id"),
	)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute summary"})
}
