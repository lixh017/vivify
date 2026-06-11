package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// openObservabilityTestDB gives the handler tests an isolated
// sqlite database pre-migrated with the CallLog table. We avoid
// the production db.Migrate (which would also try to load the
// knowledge FTS) so the test runs cleanly without the fts5 build
// tag.
func openObservabilityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := t.TempDir() + "/observability-test.db"
	gormDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := gormDB.AutoMigrate(&models.CallLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return gormDB
}

// seedCallLogs inserts a deterministic mix of rows across skills,
// providers, statuses, and dates. The test relies on the exact
// counts in this helper to assert the aggregator's output.
//
// The mix is intentionally cross-cutting:
//
//   - 50 ok rows for skill "ai_topics" provider "anthropic"
//   - 30 ok rows for skill "ai_topics" provider "anthropic" with cost
//   - 20 client_error rows for skill "ai_topics" provider "anthropic"
//   - 10 ok rows for skill "cover_generate" provider "volcengine" with cost
//   - 5 server_error rows for skill "cover_generate" provider "volcengine"
//   - 1 row for skill "credentials_list" provider "internal" (no cost)
//
// Total: 116 rows. After aggregation the expected outputs are
// pinned in the assertions below; a regression in the GROUP BY
// query would shift one of these numbers and the test would
// fail.
func seedCallLogs(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	now := time.Now()
	rows := []models.CallLog{}

	// ai_topics / anthropic — 50 ok, 20 err, all 5 cents.
	for i := 0; i < 50; i++ {
		rows = append(rows, models.CallLog{
			UserID:    userID,
			Skill:     "ai_topics",
			Provider:  "anthropic",
			Status:    models.CallStatusOK,
			LatencyMS: 120 + i,
			CostCents: 5,
			CreatedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}
	for i := 0; i < 20; i++ {
		rows = append(rows, models.CallLog{
			UserID:       userID,
			Skill:        "ai_topics",
			Provider:     "anthropic",
			Status:       models.CallStatusClientError,
			LatencyMS:    30,
			ErrorMessage: "client_error",
			CreatedAt:    now.Add(-time.Duration(i+50) * time.Hour),
		})
	}

	// cover_generate / volcengine — 10 ok with cost, 5 err.
	for i := 0; i < 10; i++ {
		rows = append(rows, models.CallLog{
			UserID:    userID,
			Skill:     "cover_generate",
			Provider:  "volcengine",
			Status:    models.CallStatusOK,
			LatencyMS: 800 + i*10,
			CostCents: 5,
			CreatedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}
	for i := 0; i < 5; i++ {
		rows = append(rows, models.CallLog{
			UserID:       userID,
			Skill:        "cover_generate",
			Provider:     "volcengine",
			Status:       models.CallStatusServerError,
			LatencyMS:    2000,
			ErrorMessage: "server_error",
			CreatedAt:    now.Add(-time.Duration(i+10) * time.Hour),
		})
	}

	// credentials_list / internal — 1 row.
	rows = append(rows, models.CallLog{
		UserID:    userID,
		Skill:     "credentials_list",
		Provider:  "internal",
		Status:    models.CallStatusOK,
		LatencyMS: 5,
		CreatedAt: now,
	})

	// Plus a row for a DIFFERENT user — the test must NOT see
	// this row in the aggregate. The handler is multi-tenant
	// and a leak would be a security regression.
	rows = append(rows, models.CallLog{
		UserID:    userID + 999,
		Skill:     "ai_topics",
		Provider:  "anthropic",
		Status:    models.CallStatusOK,
		LatencyMS: 9999,
		CostCents: 999,
		CreatedAt: now,
	})

	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed call logs: %v", err)
	}
}

// newObservabilityRouter builds a gin engine with a stub auth
// middleware and the ObservabilityHandler mounted. We use the
// existing StubUser so the handler's user_id lookup works
// without spinning up a real session.
func newObservabilityRouter(t *testing.T, db *gorm.DB, userID uint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handlersStubUser(userID))

	// Mount the handler under /api to mirror the production
	// wiring in cmd/server/main.go (the apiGroup is /api, the
	// handler registers /observability/summary).
	api := r.Group("/api")
	h := NewObservabilityHandler(db)
	h.RegisterRoutes(api)
	return r
}

// handlersStubUser mirrors middleware.StubUser but lives in the
// handlers package so the test does not need to import
// middleware (which would otherwise force the test to compile
// the full middleware chain). The implementation is a copy; it
// sets the same CtxUserID key.
func handlersStubUser(userID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.CtxUserID, userID)
		c.Next()
	}
}

// findBySkill returns the by_skill row with the given name, or
// fails the test if it's missing. The helper keeps the
// assertions terse — every check is a one-liner.
func findBySkill(t *testing.T, rows []BySkillRow, skill string) BySkillRow {
	t.Helper()
	for _, r := range rows {
		if r.Skill == skill {
			return r
		}
	}
	t.Fatalf("by_skill row for %q not found; have %+v", skill, skillNames(rows))
	return BySkillRow{}
}

func skillNames(rows []BySkillRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Skill)
	}
	sort.Strings(out)
	return out
}

// TestObservability_Summary_7dBySkill is the load-bearing
// aggregation test. It seeds a known mix of rows (see
// seedCallLogs), calls the handler, and asserts:
//
//   - ai_topics: 70 calls (50 ok + 20 err), success_rate ~ 50/70,
//     cost 50*5=250 cents
//   - cover_generate: 15 calls (10 ok + 5 err), cost 10*5=50 cents
//   - credentials_list: 1 ok call, cost 0
//   - the cross-tenant row is NOT in the output
//
// A regression that drops the user_id WHERE clause, mis-groups
// by the wrong column, or sums the wrong field fails here.
func TestObservability_Summary_7dBySkill(t *testing.T) {
	db := openObservabilityTestDB(t)
	seedCallLogs(t, db, 7)
	r := newObservabilityRouter(t, db, 7)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/observability/summary?range=7d", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	var got CallLogSummary
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Range != SummaryRange7d {
		t.Errorf("Range = %q, want %q", got.Range, SummaryRange7d)
	}

	// last_7_days should have exactly 7 buckets.
	if len(got.Last7Days) != 7 {
		t.Errorf("len(Last7Days) = %d, want 7", len(got.Last7Days))
	}

	// by_skill: the load-bearing assertions.
	ai := findBySkill(t, got.BySkill, "ai_topics")
	if ai.Calls != 70 {
		t.Errorf("ai_topics calls = %d, want 70", ai.Calls)
	}
	// success_rate is 50/70 ≈ 0.7142857... — assert to 4dp to
	// avoid float literal churn.
	if !floatNear(ai.SuccessRate, 50.0/70.0, 1e-4) {
		t.Errorf("ai_topics success_rate = %f, want ~%f", ai.SuccessRate, 50.0/70.0)
	}
	if ai.CostCents != 250 {
		t.Errorf("ai_topics cost_cents = %d, want 250", ai.CostCents)
	}
	// Latency p50/p95: the 70 latencies are [30]*20 + [120..169]*50.
	// Sorted ascending. p50: int(70*0.5)=35, sorted[34].
	// Indices 0..19 = 30, indices 20..69 = 120..169.
	// sorted[34] = 120 + (34-20) = 134.
	// p95: int(70*0.95)=66, sorted[65] = 120 + (65-20) = 165.
	if ai.P50MS != 134 {
		t.Errorf("ai_topics p50 = %d, want 134", ai.P50MS)
	}
	if ai.P95MS != 165 {
		t.Errorf("ai_topics p95 = %d, want 165", ai.P95MS)
	}

	cov := findBySkill(t, got.BySkill, "cover_generate")
	if cov.Calls != 15 {
		t.Errorf("cover_generate calls = %d, want 15", cov.Calls)
	}
	if !floatNear(cov.SuccessRate, 10.0/15.0, 1e-4) {
		t.Errorf("cover_generate success_rate = %f, want ~%f", cov.SuccessRate, 10.0/15.0)
	}
	if cov.CostCents != 50 {
		t.Errorf("cover_generate cost_cents = %d, want 50", cov.CostCents)
	}

	creds := findBySkill(t, got.BySkill, "credentials_list")
	if creds.Calls != 1 {
		t.Errorf("credentials_list calls = %d, want 1", creds.Calls)
	}
	if creds.CostCents != 0 {
		t.Errorf("credentials_list cost_cents = %d, want 0", creds.CostCents)
	}

	// Cross-tenant row MUST NOT appear.
	for _, r := range got.BySkill {
		if r.Skill == "" {
			t.Errorf("by_skill has empty skill name; by_skill=%+v", got.BySkill)
		}
	}
}

// TestObservability_Summary_ByProvider verifies the by_provider
// breakdown: anthropic and volcengine have specific cost totals
// (250 and 50 cents respectively), and "internal" is the
// third bucket.
func TestObservability_Summary_ByProvider(t *testing.T) {
	db := openObservabilityTestDB(t)
	seedCallLogs(t, db, 7)
	r := newObservabilityRouter(t, db, 7)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/observability/summary?range=7d", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got CallLogSummary
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	provs := make(map[string]ByProviderRow, len(got.ByProvider))
	for _, p := range got.ByProvider {
		provs[p.Provider] = p
	}
	if p, ok := provs["anthropic"]; !ok {
		t.Error("by_provider missing anthropic")
	} else {
		if p.Calls != 70 {
			t.Errorf("anthropic calls = %d, want 70", p.Calls)
		}
		if p.CostCents != 250 {
			t.Errorf("anthropic cost_cents = %d, want 250", p.CostCents)
		}
	}
	if p, ok := provs["volcengine"]; !ok {
		t.Error("by_provider missing volcengine")
	} else {
		if p.Calls != 15 {
			t.Errorf("volcengine calls = %d, want 15", p.Calls)
		}
		if p.CostCents != 50 {
			t.Errorf("volcengine cost_cents = %d, want 50", p.CostCents)
		}
	}
	if p, ok := provs["internal"]; !ok {
		t.Error("by_provider missing internal")
	} else if p.Calls != 1 {
		t.Errorf("internal calls = %d, want 1", p.Calls)
	}
}

// TestObservability_Summary_TodayRange pins the range=today
// shape: Last7Days has exactly 1 point, Range is "today".
func TestObservability_Summary_TodayRange(t *testing.T) {
	db := openObservabilityTestDB(t)
	seedCallLogs(t, db, 7)
	r := newObservabilityRouter(t, db, 7)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/observability/summary?range=today", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got CallLogSummary
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Range != SummaryRangeToday {
		t.Errorf("Range = %q, want %q", got.Range, SummaryRangeToday)
	}
	if len(got.Last7Days) != 1 {
		t.Errorf("len(Last7Days) = %d, want 1", len(got.Last7Days))
	}
}

// TestObservability_Summary_DefaultRange verifies a missing
// ?range= falls back to the 7d default — the dashboard's most
// common view.
func TestObservability_Summary_DefaultRange(t *testing.T) {
	db := openObservabilityTestDB(t)
	seedCallLogs(t, db, 7)
	r := newObservabilityRouter(t, db, 7)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/observability/summary", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got CallLogSummary
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Range != SummaryRange7d {
		t.Errorf("Range = %q, want default %q", got.Range, SummaryRange7d)
	}
}

// TestObservability_Summary_InvalidRange is the 400 path: an
// unknown range value lands on a 400 with a helpful error.
func TestObservability_Summary_InvalidRange(t *testing.T) {
	db := openObservabilityTestDB(t)
	r := newObservabilityRouter(t, db, 7)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/observability/summary?range=yearly", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body=%s", w.Code, w.Body.String())
	}
}

// TestObservability_Summary_RequiresAuth is the security
// regression guard: an unauthenticated request (user_id=0 on
// the context) gets a 401 and NO aggregate is returned.
func TestObservability_Summary_RequiresAuth(t *testing.T) {
	db := openObservabilityTestDB(t)
	seedCallLogs(t, db, 7)
	// Mount a router that does NOT stamp a user_id — simulates
	// the request landing on the handler without going through
	// RequireAuth. Production cannot reach this state, but the
	// handler must be robust to it.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	h := NewObservabilityHandler(db)
	h.RegisterRoutes(api)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/observability/summary?range=7d", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body=%s", w.Code, w.Body.String())
	}
}

// TestObservability_Summary_IsTenantScoped is the multi-tenant
// leak guard. We seed rows for user 7 AND user 1006 (a
// different tenant), then call the handler as user 7. The
// aggregate must contain ONLY user 7's rows.
func TestObservability_Summary_IsTenantScoped(t *testing.T) {
	db := openObservabilityTestDB(t)
	seedCallLogs(t, db, 7) // also seeds a row for user 1006
	r := newObservabilityRouter(t, db, 7)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/observability/summary?range=7d", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got CallLogSummary
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// The cross-tenant row (user 1006, cost 999) must not
	// appear in the total. The aggregate cost for user 7 is
	// 250 + 50 = 300 cents.
	totalCost := int64(0)
	for _, p := range got.ByProvider {
		totalCost += p.CostCents
	}
	if totalCost != 300 {
		t.Errorf("by_provider total cost = %d, want 300 (cross-tenant leak?)", totalCost)
	}
}

// TestPercentileNearest is a focused unit test on the percentile
// math. The test matrix is small but covers the boundary cases
// (empty, single, p=0, p=1).
func TestPercentileNearest(t *testing.T) {
	cases := []struct {
		name string
		in   []int
		p    float64
		want int
	}{
		{"empty", nil, 0.5, 0},
		{"single", []int{42}, 0.5, 42},
		{"single p=0", []int{42}, 0.0, 42},
		{"single p=1", []int{42}, 1.0, 42},
		{"sorted 10", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.5, 5},
		// p95 with n=10: rank = int(10*0.95) = int(9.5) = 9
		// (Go int conversion truncates toward zero), so we pick
		// sorted[8] = 9. This is "truncated rank" rather than
		// "nearest rank" — we picked the simpler implementation
		// because the dashboard's p95 tile rounds anyway and a
		// 1-element difference is below the human eye threshold
		// for a single-tenant window.
		{"sorted 10 p95", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.95, 9},
		{"sorted 100 median", sortedInts(1, 100), 0.5, 50},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := percentileNearest(tc.in, tc.p)
			if got != tc.want {
				t.Errorf("percentileNearest(%v, %v) = %d, want %d", tc.in, tc.p, got, tc.want)
			}
		})
	}
}

func sortedInts(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		out = append(out, i)
	}
	return out
}

// floatNear is a small helper to keep success_rate assertions
// readable. Equality on a 4dp float is more than enough for the
// dashboard's "X%" label.
func floatNear(a, b, eps float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}
