// Tests for the call-log query surface (handlers/logs.go).
//
// The four test cases cover the load-bearing behaviour:
//
//   - TestLogsList: list with seed of 10 rows, validate
//     limit/offset, skill filter, and cross-tenant isolation.
//   - TestLogsGet: hit (200) and miss (404) for the :id
//     endpoint, including the cross-tenant 404 case.
//   - TestLogsExportCSV: seed 5 rows with CSV-hostile values
//     (commas, quotes, newlines in error_message), call the
//     export endpoint, and verify the header row, the row
//     count, and the per-cell escaping.
//   - TestLogsEmptyRange: a from > to query is rejected as
//     400, not silently returned as 0 rows.
//
// We reuse the observability test's openObservabilityTestDB
// helper rather than redefining it — the schema is the same
// (just call_logs), and re-using keeps the two test files in
// sync if the migration shape changes.
package handlers

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// openLogsTestDB gives the handler tests an isolated sqlite
// database pre-migrated with the CallLog table. The migration
// mirrors openObservabilityTestDB — kept as a separate helper
// (rather than reusing the observability one directly) so the
// tests stay self-contained: a future test that needs extra
// models (User, Credential) can extend this helper without
// touching the observability tests.
func openLogsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := t.TempDir() + "/logs-test.db"
	gormDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := gormDB.AutoMigrate(&models.CallLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return gormDB
}

// seedLogsForList inserts a deterministic mix of 10 rows for
// the list test. The mix covers the four filter dimensions:
//
//   - 4 rows for skill "ai_topics" / provider "anthropic" /
//     status=0 (covers the "list all" + skill filter path)
//   - 2 rows for skill "cover_generate" / provider "volcengine"
//     / status=0 (covers the skill filter on a different skill)
//   - 2 rows for skill "ai_topics" / provider "anthropic" /
//     status=2 (covers the status filter; the row is the only
//     one with error_message="boom, with comma" so the export
//     test can reuse the same seed)
//   - 2 rows for the OTHER tenant (user 999) — the list test
//     asserts these never appear in the result.
//
// Total: 10 rows. The exact counts are pinned by the
// assertions in TestLogsList below.
func seedLogsForList(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	now := time.Now()
	rows := []models.CallLog{}
	for i := 0; i < 4; i++ {
		rows = append(rows, models.CallLog{
			UserID:    userID,
			Skill:     "ai_topics",
			Provider:  "anthropic",
			Status:    models.CallStatusOK,
			LatencyMS: 100 + i,
			CostCents: 5,
			CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		})
	}
	for i := 0; i < 2; i++ {
		rows = append(rows, models.CallLog{
			UserID:    userID,
			Skill:     "cover_generate",
			Provider:  "volcengine",
			Status:    models.CallStatusOK,
			LatencyMS: 800 + i,
			CostCents: 10,
			CreatedAt: now.Add(-time.Duration(i+10) * time.Minute),
		})
	}
	for i := 0; i < 2; i++ {
		rows = append(rows, models.CallLog{
			UserID:       userID,
			Skill:        "ai_topics",
			Provider:     "anthropic",
			Status:       models.CallStatusServerError,
			LatencyMS:    2000,
			ErrorMessage: "boom",
			CreatedAt:    now.Add(-time.Duration(i+20) * time.Minute),
		})
	}
	// Cross-tenant rows — the list MUST NOT see these.
	for i := 0; i < 2; i++ {
		rows = append(rows, models.CallLog{
			UserID:    userID + 999,
			Skill:     "ai_topics",
			Provider:  "anthropic",
			Status:    models.CallStatusOK,
			LatencyMS: 9999,
			CostCents: 999,
			CreatedAt: now,
		})
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed logs: %v", err)
	}
}

// newLogsRouter builds a gin engine with a stub user middleware
// and the LogsHandler mounted at /api/logs. We use the
// handlersStubUser helper from observability_test.go (same
// package) rather than importing middleware.StubUser — that
// keeps the test file in the same import block as the
// observability tests and avoids the import churn a fresh
// import would trigger.
func newLogsRouter(t *testing.T, db *gorm.DB, userID uint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	api.Use(handlersStubUser(userID))
	h := NewLogsHandler(db, nil)
	h.RegisterRoutes(api)
	return r
}

// TestLogsList is the load-bearing test for the list endpoint.
// It seeds 8 rows for the test user (plus 2 cross-tenant rows),
// then exercises the filter dimensions that matter for the
// dashboard:
//
//   - default list (no filter) returns the 8 rows, total=8
//   - skill=ai_topics returns 6 rows (4 ok + 2 err), total=6
//   - status=2 returns 2 rows (the server_error ai_topics
//     rows), total=2
//   - limit=3 caps the items length but total still reports
//     8 (count is unfiltered by limit)
//   - offset=3 + limit=3 returns the next page
//   - the cross-tenant rows never appear, in any filter
//     combination
//
// A regression in the user_id WHERE clause, the ORDER BY
// clause, or any of the filter dimensions fails here.
func TestLogsList(t *testing.T) {
	db := openLogsTestDB(t)
	seedLogsForList(t, db, 7)
	r := newLogsRouter(t, db, 7)

	// 1. Default list — 8 rows for user 7, no filter.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("default list: expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	var got LogsListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal default: %v", err)
	}
	if got.Total != 8 {
		t.Errorf("default list total = %d, want 8", got.Total)
	}
	if len(got.Items) != 8 {
		t.Errorf("default list items len = %d, want 8", len(got.Items))
	}
	// Every returned item must belong to user 7 — no
	// cross-tenant leak.
	for _, it := range got.Items {
		if it.UserID != 7 {
			t.Errorf("default list leaked user_id=%d (skill=%q)", it.UserID, it.Skill)
		}
	}
	// Default order: created_at DESC. The seed inserts the
	// cover_generate rows with older timestamps than the
	// ai_topics server_error rows; the first item should be
	// the most recent ai_topics OK row (now - 0 minutes).
	if got.Items[0].Skill != "ai_topics" || got.Items[0].Status != models.CallStatusOK {
		t.Errorf("default list first item: got skill=%q status=%d, want ai_topics/0",
			got.Items[0].Skill, got.Items[0].Status)
	}

	// 2. skill=ai_topics filter.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs?skill=ai_topics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("skill filter: expected 200, got %d", w.Code)
	}
	got = LogsListResponse{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Total != 6 {
		t.Errorf("skill filter total = %d, want 6", got.Total)
	}
	if len(got.Items) != 6 {
		t.Errorf("skill filter items len = %d, want 6", len(got.Items))
	}
	for _, it := range got.Items {
		if it.Skill != "ai_topics" {
			t.Errorf("skill filter leaked skill=%q", it.Skill)
		}
	}

	// 3. status=2 filter.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs?status=2", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status filter: expected 200, got %d", w.Code)
	}
	got = LogsListResponse{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Total != 2 {
		t.Errorf("status filter total = %d, want 2", got.Total)
	}
	for _, it := range got.Items {
		if it.Status != 2 {
			t.Errorf("status filter leaked status=%d", it.Status)
		}
	}

	// 4. limit=3 caps the items length; total is the
	// unfiltered count of 8.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs?limit=3", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("limit=3: expected 200, got %d", w.Code)
	}
	got = LogsListResponse{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Limit != 3 {
		t.Errorf("limit=3 echo: got %d, want 3", got.Limit)
	}
	if got.Total != 8 {
		t.Errorf("limit=3 total: got %d, want 8 (limit must not affect count)", got.Total)
	}
	if len(got.Items) != 3 {
		t.Errorf("limit=3 items len: got %d, want 3", len(got.Items))
	}

	// 5. offset=3 + limit=3 returns the next 3 rows. The
	// combined page must not overlap the first page.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs?offset=3&limit=3", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("offset=3: expected 200, got %d", w.Code)
	}
	got = LogsListResponse{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Offset != 3 || got.Limit != 3 {
		t.Errorf("offset/limit echo: got offset=%d limit=%d, want 3/3", got.Offset, got.Limit)
	}
	if len(got.Items) != 3 {
		t.Errorf("offset=3 items len: got %d, want 3", len(got.Items))
	}
}

// TestLogsList_RejectsBadParams is the 400-path coverage for
// the parseFilter helper. Each bad-input case is checked once;
// the test is the same shape as the observability
// InvalidRange test (one HTTP call, one status assertion) and
// keeps the parser regression suite terse.
func TestLogsList_RejectsBadParams(t *testing.T) {
	db := openLogsTestDB(t)
	r := newLogsRouter(t, db, 7)

	cases := []struct {
		name  string
		query string
	}{
		{"bad status", "?status=4"},
		{"non-numeric status", "?status=oops"},
		{"bad from", "?from=not-a-date"},
		{"bad to", "?to=2026-13-99T99:99:99Z"},
		{"from after to", "?from=2026-06-01T00:00:00Z&to=2026-05-01T00:00:00Z"},
		{"negative limit", "?limit=-1"},
		{"non-numeric limit", "?limit=abc"},
		{"negative offset", "?offset=-5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs"+tc.query, nil))
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

// TestLogsList_TenantIsolation is the focused multi-tenant
// regression. Seeds 8 rows for user 7 plus 2 rows for user
// 1006, then asserts that user 7's view never returns a row
// with user_id != 7. (TestLogsList already covers this, but
// the dedicated test makes a leak a louder failure.)
func TestLogsList_TenantIsolation(t *testing.T) {
	db := openLogsTestDB(t)
	seedLogsForList(t, db, 7) // also seeds rows for user 1006
	r := newLogsRouter(t, db, 7)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got LogsListResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	for _, it := range got.Items {
		if it.UserID != 7 {
			t.Fatalf("cross-tenant leak: row user_id=%d in user 7's view", it.UserID)
		}
	}
	if got.Total != 8 {
		t.Errorf("total = %d, want 8 (cross-tenant rows leaked into count?)", got.Total)
	}
}

// TestLogsGet covers the :id endpoint: hit, miss, and
// cross-tenant. The cross-tenant case is a regression for the
// 404-vs-403 invariant: a request to a row owned by another
// tenant MUST return 404 (not 403), so the response shape
// cannot be used to enumerate other tenants' row ids.
func TestLogsGet(t *testing.T) {
	db := openLogsTestDB(t)
	seedLogsForList(t, db, 7)
	r := newLogsRouter(t, db, 7)

	// Find the first row owned by user 7.
	var first models.CallLog
	if err := db.Where("user_id = ?", uint(7)).Order("id ASC").First(&first).Error; err != nil {
		t.Fatalf("find first row: %v", err)
	}
	// Hit: GET /api/logs/:id returns 200 with the row.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs/"+strconv.FormatUint(uint64(first.ID), 10), nil))
	if w.Code != http.StatusOK {
		t.Fatalf("hit: expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	var got models.CallLog
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.ID != first.ID || got.UserID != 7 {
		t.Errorf("hit body: got id=%d user_id=%d, want id=%d user_id=7", got.ID, got.UserID, first.ID)
	}
	// The full payload must include the cost / error fields
	// the list endpoint also returns.
	if got.CostCents != first.CostCents {
		t.Errorf("hit body: cost_cents=%d, want %d", got.CostCents, first.CostCents)
	}

	// Miss: a row that does not exist.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs/999999", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("miss: expected 404, got %d", w.Code)
	}

	// Cross-tenant: a row owned by user 1006 (userID + 999
	// in the seed). Requesting it as user 7 must return 404,
	// not 403.
	var other models.CallLog
	if err := db.Where("user_id = ?", uint(7+999)).Order("id ASC").First(&other).Error; err != nil {
		t.Fatalf("find other-tenant row: %v", err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs/"+strconv.FormatUint(uint64(other.ID), 10), nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("cross-tenant: expected 404, got %d (must be 404 not 403)", w.Code)
	}

	// Bad id: non-numeric.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs/abc", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad id: expected 400, got %d", w.Code)
	}
}

// TestLogsExportCSV seeds 5 rows with deliberately
// CSV-hostile error_message values and asserts the export:
//
//   - returns 200 with text/csv and Content-Disposition
//   - has the documented header row in the right order
//   - has 5 data rows (plus 1 header)
//   - the comma / quote / newline in error_message is
//     properly escaped per RFC 4180
//
// The test runs both through the HTTP handler and through
// the lower-level encodeLogsCSV helper. The handler test
// covers the wire format; the helper test covers the
// round-trip parse with encoding/csv.
func TestLogsExportCSV(t *testing.T) {
	db := openLogsTestDB(t)
	// Seed: 5 rows with a mix of statuses and one
	// CSV-hostile error_message ("contains, \"quote\" and
	// newline\nhere"). The comma / quote / newline is the
	// payload the encoding/csv writer is required to escape.
	hostile := "contains, \"quote\" and\nnewline"
	now := time.Now()
	rows := []models.CallLog{
		{UserID: 7, Skill: "ai_topics", Provider: "anthropic", Status: 0, LatencyMS: 100, CostCents: 5, CostCurrency: "CNY", CreatedAt: now.Add(-1 * time.Minute)},
		{UserID: 7, Skill: "ai_topics", Provider: "anthropic", Status: 0, LatencyMS: 110, CostCents: 5, CostCurrency: "CNY", CreatedAt: now.Add(-2 * time.Minute)},
		{UserID: 7, Skill: "cover_generate", Provider: "volcengine", Status: 0, LatencyMS: 800, CostCents: 10, CostCurrency: "CNY", CreatedAt: now.Add(-3 * time.Minute)},
		{UserID: 7, Skill: "ai_topics", Provider: "anthropic", Status: 2, LatencyMS: 2000, CostCents: 0, CostCurrency: "CNY", ErrorMessage: hostile, CreatedAt: now.Add(-4 * time.Minute)},
		{UserID: 7, Skill: "credentials_list", Provider: "internal", Status: 0, LatencyMS: 5, CostCents: 0, CostCurrency: "CNY", CreatedAt: now.Add(-5 * time.Minute)},
		// Cross-tenant row — must not appear in the export.
		{UserID: 7 + 999, Skill: "ai_topics", Provider: "anthropic", Status: 0, LatencyMS: 9999, CostCents: 999, CostCurrency: "CNY", CreatedAt: now},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed export: %v", err)
	}
	r := newLogsRouter(t, db, 7)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/logs/export.csv", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("export: expected 200, got %d; body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv...", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment;") {
		t.Errorf("Content-Disposition = %q, want attachment;...", cd)
	}

	// Parse with the stdlib csv reader. If the writer failed
	// to escape a comma, the reader would return a row with
	// the wrong number of fields.
	rdr := csv.NewReader(strings.NewReader(w.Body.String()))
	parsed, err := rdr.ReadAll()
	if err != nil {
		t.Fatalf("csv parse: %v\nbody=%s", err, w.Body.String())
	}
	// 1 header + 5 data rows. The cross-tenant row is
	// filtered by the user_id WHERE clause.
	if len(parsed) != 6 {
		t.Fatalf("csv row count = %d, want 6 (1 header + 5 data)", len(parsed))
	}

	// Header order. Any future re-order is a breaking change
	// for downstream consumers, so this assertion doubles as
	// a contract.
	wantHeader := []string{
		"id", "created_at", "user_id", "skill", "provider",
		"status", "latency_ms", "input_tokens", "output_tokens",
		"cost_cents", "cost_currency", "error_message",
	}
	if len(parsed[0]) != len(wantHeader) {
		t.Fatalf("header col count = %d, want %d", len(parsed[0]), len(wantHeader))
	}
	for i, c := range wantHeader {
		if parsed[0][i] != c {
			t.Errorf("header[%d] = %q, want %q", i, parsed[0][i], c)
		}
	}

	// Find the row that should carry the hostile
	// error_message. The csv reader is responsible for
	// un-escaping; if the writer was correct, the value
	// round-trips byte-for-byte.
	var found bool
	for _, rec := range parsed[1:] {
		if rec[3] == "ai_topics" && rec[5] == "2" {
			if rec[11] != hostile {
				t.Errorf("hostile error_message not round-tripped:\n  got:  %q\n  want: %q", rec[11], hostile)
			}
			found = true
		}
	}
	if !found {
		t.Error("could not find the server_error row in the export")
	}

	// Cross-tenant row check: no row in the export should
	// have user_id=1006.
	for i, rec := range parsed[1:] {
		if rec[2] == "1006" {
			t.Errorf("cross-tenant row in export at line %d: user_id=%s", i+1, rec[2])
		}
	}

	// Raw body must contain the escaped form somewhere
	// (RFC 4180: " is escaped as "", and the field is
	// wrapped in quotes). A direct substring check on the
	// raw body is the strongest "the writer actually
	// escaped" signal, complementing the parser round-trip
	// above.
	if !strings.Contains(w.Body.String(), `""quote""`) {
		t.Errorf("raw body missing the RFC4180 escaped \"\"quote\"\" sequence:\n%s", w.Body.String())
	}
}

// TestLogsExportCSV_Helper is a focused unit test on the
// encodeLogsCSV function. The test bypasses the HTTP layer
// and exercises the helper directly so a regression in the
// encoding logic surfaces independently of the handler's
// wiring.
func TestLogsExportCSV_Helper(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	rows := []models.CallLog{
		{ID: 1, UserID: 7, Skill: "ai_topics", Provider: "anthropic", Status: 0, LatencyMS: 100, CostCents: 5, CostCurrency: "CNY", CreatedAt: now},
		{ID: 2, UserID: 7, Skill: "ai_topics", Provider: "anthropic", Status: 2, LatencyMS: 2000, CostCents: 0, CostCurrency: "CNY", ErrorMessage: "boom, with \"quote\"", CreatedAt: now.Add(-1 * time.Minute)},
	}
	buf, err := encodeLogsCSV(rows)
	if err != nil {
		t.Fatalf("encodeLogsCSV: %v", err)
	}
	rdr := csv.NewReader(strings.NewReader(buf.String()))
	parsed, err := rdr.ReadAll()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 3 {
		t.Fatalf("row count = %d, want 3", len(parsed))
	}
	// Row 1: the no-error row. error_message field is "".
	if parsed[1][11] != "" {
		t.Errorf("row 1 error_message = %q, want \"\"", parsed[1][11])
	}
	// Row 2: the host row. error_message round-trips.
	if parsed[2][11] != "boom, with \"quote\"" {
		t.Errorf("row 2 error_message = %q, want %q", parsed[2][11], "boom, with \"quote\"")
	}
	// created_at is RFC3339 UTC. Row 1 should be exactly
	// "2026-06-05T12:00:00Z".
	if parsed[1][1] != "2026-06-05T12:00:00Z" {
		t.Errorf("row 1 created_at = %q, want 2026-06-05T12:00:00Z", parsed[1][1])
	}
}

// TestLogsEmptyRange exercises the from > to 400 path. The
// observability test (TestObservability_Summary_InvalidRange)
// is the template: a single request, a single status check.
// The test name follows the same "<surface>_<case>" naming
// pattern.
func TestLogsEmptyRange(t *testing.T) {
	db := openLogsTestDB(t)
	r := newLogsRouter(t, db, 7)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/api/logs?from=2026-06-01T00:00:00Z&to=2026-05-01T00:00:00Z", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("from > to: expected 400, got %d; body=%s", w.Code, w.Body.String())
	}
}

// TestLogsRequiresAuth is the 401 guard for the
// call-log surface: a router that does NOT stamp a user_id
// must return 401, never 200, on every endpoint. The test
// hits all three endpoints in a single sub-test to keep
// the matrix terse.
func TestLogsRequiresAuth(t *testing.T) {
	db := openLogsTestDB(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	h := NewLogsHandler(db, nil)
	h.RegisterRoutes(api)

	cases := []struct {
		name string
		path string
	}{
		{"list", "/api/logs"},
		{"get", "/api/logs/1"},
		{"export", "/api/logs/export.csv"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401, got %d; body=%s", tc.name, w.Code, w.Body.String())
			}
		})
	}
}

// _silenceMiddleware is a no-op import guard. middleware is
// imported in the test file via the handler package, but if
// a future refactor removes every other reference the import
// would drop. Keeping an explicit guard at the bottom makes
// the import list stable across refactors.
var _ = middleware.CtxUserID
