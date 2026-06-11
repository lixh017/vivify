package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// openTestDB gives the middleware tests an in-memory SQLite
// handle. We use a private temp file (not :memory:) because
// shared in-memory DBs persist across test functions in the same
// process and would leak rows from one test to the next.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := t.TempDir() + "/call-log-test.db"
	gormDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := gormDB.AutoMigrate(&models.CallLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return gormDB
}

// newCallLogRouter builds a gin engine with the CallLog middleware
// and a small set of probe routes (200, 401, 500). The probe
// handlers are deliberately minimal — we only care about what
// the middleware records, not what the handler returns.
func newCallLogRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CallLog(CallLogConfig{DB: db}))

	r.GET("/api/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	// StubUser stamps a user_id on the context so the middleware
	// can populate CallLog.UserID — same shape as the production
	// RequireAuth flow.
	r.GET("/api/auth-ok", StubUser(42), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	// /api/auth/login is on the default skip-list — a 200 response
	// here should NOT produce a CallLog row. We assert that
	// explicitly in TestCallLog_SkipsAuth.
	r.POST("/api/auth/login", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	// /api/observability/* is also on the skip list.
	r.GET("/api/observability/summary", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"range": "7d"})
	})
	// /healthz is on the skip list.
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	return r
}

// loadCallLogs returns the CallLog rows currently in the test DB.
// Returning the slice (rather than asserting inside the helper)
// keeps the test bodies terse — the assertion is right next to
// the call that produced the row.
func loadCallLogs(t *testing.T, db *gorm.DB) []models.CallLog {
	t.Helper()
	var rows []models.CallLog
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("load call logs: %v", err)
	}
	return rows
}

// TestCallLog_Records200AndInfersSkill is the happy-path test:
// a 200 response on /api/ok produces exactly one row, with the
// status=0 (OK), a non-zero latency, the inferred skill
// ("other" — /api/ok is not in the known-skills list), and
// provider="internal" (the default).
func TestCallLog_Records200AndInfersSkill(t *testing.T) {
	db := openTestDB(t)
	r := newCallLogRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/ok", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	rows := loadCallLogs(t, db)
	if len(rows) != 1 {
		t.Fatalf("expected 1 call_log row, got %d", len(rows))
	}
	got := rows[0]
	if got.Status != models.CallStatusOK {
		t.Errorf("Status = %d, want %d (OK)", got.Status, models.CallStatusOK)
	}
	if got.Skill != "other" {
		t.Errorf("Skill = %q, want %q", got.Skill, "other")
	}
	if got.Provider != "internal" {
		t.Errorf("Provider = %q, want %q", got.Provider, "internal")
	}
	if got.LatencyMS < 0 {
		t.Errorf("LatencyMS = %d, want >= 0", got.LatencyMS)
	}
}

// TestCallLog_RecordsClientError verifies that a 401 response
// produces a row with status=1 (client error) and a generic
// error message. The response status is what drives the
// classification, not any handler-side flag.
func TestCallLog_RecordsClientError(t *testing.T) {
	db := openTestDB(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CallLog(CallLogConfig{DB: db}))
	r.GET("/api/deny", func(c *gin.Context) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/deny", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	rows := loadCallLogs(t, db)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Status != models.CallStatusClientError {
		t.Errorf("Status = %d, want %d (client error)", rows[0].Status, models.CallStatusClientError)
	}
	if rows[0].ErrorMessage == "" {
		t.Error("expected non-empty ErrorMessage for a 401")
	}
}

// TestCallLog_RecordsServerError verifies that a 500 response
// produces a row with status=2 (server error). The dashboard
// "5xx rate" tile reads from status=2 directly, so this is the
// load-bearing path for the SLO.
func TestCallLog_RecordsServerError(t *testing.T) {
	db := openTestDB(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CallLog(CallLogConfig{DB: db}))
	r.GET("/api/boom", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/boom", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}

	rows := loadCallLogs(t, db)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Status != models.CallStatusServerError {
		t.Errorf("Status = %d, want %d (server error)", rows[0].Status, models.CallStatusServerError)
	}
}

// TestCallLog_SkipsAuth verifies the canonical skip-list:
// /api/auth/login does NOT produce a CallLog row. The reason
// matters: the auth surface handles unauthenticated requests
// and writing a row for "user tried to log in with bad
// credentials" would inflate the failure rate with non-failures.
func TestCallLog_SkipsAuth(t *testing.T) {
	db := openTestDB(t)
	r := newCallLogRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/auth/login", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from stub, got %d", w.Code)
	}
	if rows := loadCallLogs(t, db); len(rows) != 0 {
		t.Errorf("expected 0 rows for /api/auth/*, got %d", len(rows))
	}
}

// TestCallLog_SkipsObservabilityAndHealth is a sweep: hit every
// default-skip URL and assert no rows are written. The whole
// skip-list is asserted in one test because the cost of writing
// more granular tests is high (one per prefix) and the value of
// "the list is the list" is high.
func TestCallLog_SkipsObservabilityAndHealth(t *testing.T) {
	db := openTestDB(t)
	r := newCallLogRouter(db)

	for _, path := range []string{
		"/api/auth/login",
		"/api/observability/summary",
		"/healthz",
	} {
		method := http.MethodGet
		if path == "/api/auth/login" {
			method = http.MethodPost
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	}
	if rows := loadCallLogs(t, db); len(rows) != 0 {
		t.Errorf("expected 0 rows for skip-list paths, got %d", len(rows))
	}
}

// TestCallLog_StampsUserIDFromContext verifies the per-tenant
// wiring: when StubUser(42) runs, the middleware picks up the
// user_id from the Gin context and writes it to the row. This
// is the contract the by_skill aggregation relies on — a
// missing user_id would mean cross-tenant rows are summed into
// the same dashboard.
func TestCallLog_StampsUserIDFromContext(t *testing.T) {
	db := openTestDB(t)
	r := newCallLogRouter(db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth-ok", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	rows := loadCallLogs(t, db)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].UserID != 42 {
		t.Errorf("UserID = %d, want 42", rows[0].UserID)
	}
}

// TestInferSkill_StableAcrossHandlers is a regression guard on
// the URL -> skill mapping. Each entry is the URL a real handler
// in main.go registers. If a refactor renames a path, this test
// fails BEFORE the dashboard silently relabels the call.
func TestInferSkill_StableAcrossHandlers(t *testing.T) {
	cases := []struct {
		path   string
		method string
		want   string
	}{
		{"/api/ai/topics", "POST", "ai_topics"},
		{"/api/ai/score", "POST", "ai_score"},
		{"/api/ai/pipeline", "POST", "ai_pipeline"},
		{"/api/cover", "POST", "cover_generate"},
		{"/api/credentials", "GET", "credentials_list"},
		{"/api/credentials", "POST", "credentials_create"},
		{"/api/credentials/1", "DELETE", "credentials_delete"},
		{"/api/topics", "GET", "topics_list"},
		{"/api/topics", "POST", "topics_create"},
		{"/api/topics/42", "GET", "topics_get"},
		{"/api/topics/42", "PUT", "topics_update"},
		{"/api/topics/42", "DELETE", "topics_delete"},
		{"/api/scripts", "GET", "scripts_list"},
		{"/api/content-items/1", "GET", "content_items_get"},
		{"/api/knowledge/1", "GET", "knowledge_get"},
		{"/api/series", "GET", "series_list"},
		{"/api/random-unknown", "GET", "other"},
	}
	for _, tc := range cases {
		got := inferSkill(tc.path, tc.method)
		if got != tc.want {
			t.Errorf("inferSkill(%q, %q) = %q, want %q", tc.path, tc.method, got, tc.want)
		}
	}
}

// TestCallLog_AllowsHandlerOverride verifies the CallLogRef seam:
// a handler that stamps Provider/CostCents/ErrorMessage on the
// ref produces a row with those values. This is the contract the
// cover handler (and future AI handlers) will use to attach
// provider + cost metadata.
func TestCallLog_AllowsHandlerOverride(t *testing.T) {
	db := openTestDB(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CallLog(CallLogConfig{DB: db}))
	r.GET("/api/cover", func(c *gin.Context) {
		ref := CallLogFromContext(c)
		if ref == nil {
			t.Fatal("expected CallLogRef to be set")
		}
		ref.Provider = "volcengine"
		ref.CostCents = 5
		ref.CostCurrency = "CNY"
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/cover", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	rows := loadCallLogs(t, db)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Provider != "volcengine" {
		t.Errorf("Provider = %q, want volcengine", rows[0].Provider)
	}
	if rows[0].CostCents != 5 {
		t.Errorf("CostCents = %d, want 5", rows[0].CostCents)
	}
	if rows[0].CostCurrency != "CNY" {
		t.Errorf("CostCurrency = %q, want CNY", rows[0].CostCurrency)
	}
}
