package handlers

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// BillingHandler exposes the MCN's monthly cost / usage
// surface. Phase 4 #105 — the minimal P1: one month
// summary (calls / cost / projection) plus a CSV export.
// Pricing changes do not retroactively rewrite call_log,
// so we always recompute from the historical
// cost_cents / cost_units rows rather than re-multiplying
// against today's price.
type BillingHandler struct {
	db *gorm.DB
}

// NewBillingHandler wires a BillingHandler. db is the same
// gorm handle every other handler uses; the handler does
// not maintain its own state.
func NewBillingHandler(db *gorm.DB) *BillingHandler {
	return &BillingHandler{db: db}
}

// RegisterRoutes attaches the billing endpoints. Both are
// mounted under the protected /api group so the call_log
// middleware picks them up (we exclude the path via the
// middleware's skip-list to keep the billing query from
// logging itself).
func (h *BillingHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/billing/summary", h.MonthSummary)
	r.GET("/billing/export.csv", h.MonthExport)
}

// monthBounds returns [start, endExclusive) for the given
// YYYY-MM string. "today" or an empty string is the
// current month. Invalid input falls back to the current
// month so a misconfigured client still gets a useful
// response.
func monthBounds(month string) (time.Time, time.Time) {
	if month == "" || month == "today" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()),
			time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
	}
	t, err := time.Parse("2006-01", month)
	if err != nil {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()),
			time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()),
		time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
}

// MonthSummary — GET /api/billing/summary?month=YYYY-MM
//
// 200: { month, calls, cost_cents, by_skill[], by_provider[], projection_cents }
// 500: db failure
//
// The summary is multi-tenant scoped: every aggregate
// filters on user_id from the gin context. The projection
// is a linear extrapolation of the current month's spend
// rate at the time of the call; it is intentionally
// dumb (no seasonal adjustment) so the operator can see
// "running too hot this month" without it being hidden
// behind a smoothing algorithm.
func (h *BillingHandler) MonthSummary(c *gin.Context) {
	month := c.Query("month")
	start, end := monthBounds(month)

	userID := UserIDFromContext(c)
	scope := h.db.WithContext(c.Request.Context()).Model(&models.CallLog{}).
		Where("created_at >= ? AND created_at < ?", start, end)
	if userID != 0 {
		scope = scope.Where("user_id = ?", userID)
	}

	// Headline KPIs.
	type kpi struct {
		Calls     int64 `json:"calls"`
		CostCents int64 `json:"cost_cents"`
	}
	var k kpi
	if err := scope.Select("COUNT(*) AS calls, COALESCE(SUM(cost_cents), 0) AS cost_cents").
		Scan(&k).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "billing query failed"})
		return
	}

	// by_skill: a small in-Go aggregation. The call_log table
	// is already grouped by skill per the observability
	// endpoint, but the billing surface wants a flat
	// (skill, calls, cost) tuple so the operator can build
	// a stacked chart without re-querying.
	type skillRow struct {
		Skill     string
		Calls     int64
		CostCents int64
	}
	var skillRows []skillRow
	if err := scope.Select("skill, COUNT(*) AS calls, COALESCE(SUM(cost_cents), 0) AS cost_cents").
		Group("skill").Order("calls DESC").Scan(&skillRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "billing by-skill query failed"})
		return
	}
	bySkill := make([]gin.H, 0, len(skillRows))
	for _, r := range skillRows {
		bySkill = append(bySkill, gin.H{
			"skill":      r.Skill,
			"calls":      r.Calls,
			"cost_cents": r.CostCents,
		})
	}

	// by_provider: same shape, different group-by.
	type providerRow struct {
		Provider  string
		Calls     int64
		CostCents int64
	}
	var providerRows []providerRow
	if err := scope.Select("provider, COUNT(*) AS calls, COALESCE(SUM(cost_cents), 0) AS cost_cents").
		Group("provider").Order("calls DESC").Scan(&providerRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "billing by-provider query failed"})
		return
	}
	byProvider := make([]gin.H, 0, len(providerRows))
	for _, r := range providerRows {
		byProvider = append(byProvider, gin.H{
			"provider":   r.Provider,
			"calls":      r.Calls,
			"cost_cents": r.CostCents,
		})
	}

	// Linear projection: cost_so_far * (month_length / days_elapsed).
	// days_elapsed >= 1 (we never project over a zero-day window).
	now := time.Now()
	daysInMonth := end.Sub(start).Hours() / 24
	daysElapsed := now.Sub(start).Hours() / 24
	if daysElapsed < 1 {
		daysElapsed = 1
	}
	if daysInMonth < 1 {
		daysInMonth = 1
	}
	projection := int64(float64(k.CostCents) * (daysInMonth / daysElapsed))

	c.JSON(http.StatusOK, gin.H{
		"month":            start.Format("2006-01"),
		"calls":            k.Calls,
		"cost_cents":       k.CostCents,
		"days_elapsed":     int(daysElapsed),
		"days_in_month":    int(daysInMonth),
		"projection_cents": projection,
		"by_skill":         bySkill,
		"by_provider":      byProvider,
	})
}

// MonthExport — GET /api/billing/export.csv?month=YYYY-MM
//
// 200: text/csv with columns
//   id,created_at,user_id,skill,provider,status,
//   latency_ms,input_tokens,output_tokens,cost_cents,cost_units,cost_currency
//
// Streamed via bytes.Buffer because the row count is bounded
// by 1 month's calls; at the MCN rate (~10k calls/day) the
// CSV is well under 10 MB. A future iteration can switch to
// c.Stream for arbitrarily large windows.
func (h *BillingHandler) MonthExport(c *gin.Context) {
	month := c.Query("month")
	start, end := monthBounds(month)

	userID := UserIDFromContext(c)
	q := h.db.WithContext(c.Request.Context()).Model(&models.CallLog{}).
		Where("created_at >= ? AND created_at < ?", start, end).
		Order("created_at ASC")
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	rows, err := q.Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "billing export query failed"})
		return
	}
	defer rows.Close()

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{
		"id", "created_at", "user_id", "skill", "provider",
		"status", "latency_ms", "input_tokens", "output_tokens",
		"cost_cents", "cost_units", "cost_currency",
	})
	for rows.Next() {
		var r models.CallLog
		if err := h.db.ScanRows(rows, &r); err != nil {
			continue
		}
		_ = w.Write([]string{
			strconv.Itoa(int(r.ID)),
			r.CreatedAt.Format(time.RFC3339),
			strconv.Itoa(int(r.UserID)),
			r.Skill,
			r.Provider,
			strconv.Itoa(r.Status),
			strconv.Itoa(r.LatencyMS),
			strconv.Itoa(r.InputTokens),
			strconv.Itoa(r.OutputTokens),
			strconv.Itoa(r.CostCents),
			strconv.Itoa(r.CostUnits),
			r.CostCurrency,
		})
	}
	w.Flush()

	c.Header("Content-Type", "text/csv; charset=utf-8")
	// Filename: opc-billing-YYYY-MM.csv
	c.Header("Content-Disposition",
		"attachment; filename=opc-billing-"+start.Format("2006-01")+".csv")
	c.Data(http.StatusOK, "text/csv", buf.Bytes())
}

// "今天的该日" 占位 — MonthSummary JSON 字段命名兼容 console 端
// BillingSummary 接口的 hour_offset (被预留给 Phase 4+ 的"按小时粒度")。
// 暂时返回 0, 后续若加 per-hour billing, 可在这里加一个 sum 子查询。
var _ = strings.HasPrefix // silence unused import if hooks trim later

// middleware stub (avoids import cycle): the call_log
// skip-list lives in middleware/call_log.go and is set by
// main.go at boot. Importing the package here would cause a
// cycle; the constant is referenced for documentation only.
var _ = middleware.CallLogConfig{}
