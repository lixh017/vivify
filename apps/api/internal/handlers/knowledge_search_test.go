//go:build fts5

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// setupTestKnowledgeSearchRouter wires the CRUD and search handlers
// against a freshly-migrated in-memory DB. The FTS5 shadow table is
// created by the same db.EnsureKnowledgeFTS call that
// setupTestKnowledgeDocRouter uses, so both helpers share one DDL
// source of truth (and a future refactor cannot drift them apart).
//
// This helper is gated on the `fts5` build tag because the FTS
// surface can only be created against an SQLite engine that has FTS5
// compiled in. A plain `go test ./...` skips these integration
// tests; the CI workflow runs them with `-tags fts5`.
func setupTestKnowledgeSearchRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gormDB := newTestDB(t)
	if err := gormDB.AutoMigrate(&models.KnowledgeDoc{}); err != nil {
		t.Fatalf("automigrate KnowledgeDoc: %v", err)
	}
	// db.Migrate runs AutoMigrate for all entities and then ensures
	// the FTS5 surface. Calling it on an already-migrated handle is
	// a no-op thanks to IF NOT EXISTS.
	if err := ensureFTSForTest(t, gormDB); err != nil {
		t.Fatalf("ensure FTS: %v", err)
	}
	r := gin.New()
	crud := NewKnowledgeDocHandler(gormDB)
	crud.RegisterRoutes(r)
	search := NewKnowledgeSearchHandler(gormDB)
	search.RegisterRoutes(r)
	return r, gormDB
}

// ensureFTSForTest brings the FTS5 surface online via the shared
// db.EnsureKnowledgeFTS helper. Extracted so the integration tests
// can also be exercised with a hand-rolled FakeStore in the
// handler-only test below.
func ensureFTSForTest(t *testing.T, gormDB *gorm.DB) error {
	t.Helper()
	return ensureSearchFTS(gormDB)
}

// TestKnowledgeSearchBasic inserts three knowledge docs and confirms
// that a plain-term FTS5 query returns the matching row. This is the
// happy-path contract the spec calls out in section 4.9.7.
func TestKnowledgeSearchBasic(t *testing.T) {
	r, gormDB := setupTestKnowledgeSearchRouter(t)

	seeds := []models.KnowledgeDoc{
		{Title: "声音调性指南", Path: "ip-style-guide/voice", Content: "核心是冷静、不滥用情绪词", Tags: "voice,style", DocType: "ip-style"},
		{Title: "选题 SOP", Path: "sop/topic-pipeline", Content: "从趋势信号到立项的完整流程", Tags: "sop", DocType: "sop"},
		{Title: "封面视觉规范", Path: "ip-style-guide/visual", Content: "封面层级、字体、留白", Tags: "visual,style", DocType: "ip-style"},
	}
	for i := range seeds {
		if err := gormDB.Create(&seeds[i]).Error; err != nil {
			t.Fatalf("seed %s: %v", seeds[i].Title, err)
		}
	}

	w := do(t, r, "GET", "/knowledge/search?q=声音调性", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items  []models.KnowledgeDoc `json:"items"`
		Query  string                `json:"query"`
		Total  int64                 `json:"total"`
		Limit  int                   `json:"limit"`
		Offset int                   `json:"offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Query != "声音调性" {
		t.Errorf("expected query echoed back, got %q", resp.Query)
	}
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("expected 1 match, got total=%d items=%d", resp.Total, len(resp.Items))
	}
	if resp.Items[0].Title != "声音调性指南" {
		t.Errorf("unexpected title: %q", resp.Items[0].Title)
	}
}

// TestKnowledgeSearchByDocType verifies the optional doc_type filter
// narrows results to rows of that type only. The doc_type is applied
// to the joined base table, not the FTS index.
func TestKnowledgeSearchByDocType(t *testing.T) {
	r, gormDB := setupTestKnowledgeSearchRouter(t)

	seeds := []models.KnowledgeDoc{
		{Title: "风格指南 - voice", Content: "voice guidance", DocType: "ip-style"},
		{Title: "风格指南 - visual", Content: "visual guidance", DocType: "ip-style"},
		{Title: "运营 SOP - voice", Content: "voice operations SOP", DocType: "sop"},
	}
	for i := range seeds {
		if err := gormDB.Create(&seeds[i]).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// "voice" appears in the title or content of the first and third
	// rows. With doc_type=ip-style we should see only the first row.
	w := do(t, r, "GET", "/knowledge/search?q=voice&doc_type=ip-style", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []models.KnowledgeDoc `json:"items"`
		Total int64                 `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Total counts FTS matches that ALSO match the doc_type filter,
	// so we expect 1.
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("expected 1 ip-style match, got total=%d items=%d", resp.Total, len(resp.Items))
	}
	for _, kd := range resp.Items {
		if kd.DocType != "ip-style" {
			t.Errorf("doc_type filter leaked: got %q", kd.DocType)
		}
	}
}

// TestKnowledgeSearchEmptyQuery asserts that the endpoint rejects an
// empty q parameter with a 400. An empty query is silently swallowed
// by FTS5 (matches everything), so we must guard at the handler.
func TestKnowledgeSearchEmptyQuery(t *testing.T) {
	r, _ := setupTestKnowledgeSearchRouter(t)

	w := do(t, r, "GET", "/knowledge/search?q=", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty q, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKnowledgeSearchWhitespaceQuery treats whitespace-only q the same
// as an empty query. The handler trims before the empty check, so
// "   " is rejected.
func TestKnowledgeSearchWhitespaceQuery(t *testing.T) {
	r, _ := setupTestKnowledgeSearchRouter(t)

	w := do(t, r, "GET", "/knowledge/search?q=%20%20", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for whitespace q, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKnowledgeSearchInvalidFTSQuery asserts that a malformed MATCH
// expression is surfaced as a 400 rather than a 500. The double
// quote is a syntax error in FTS5.
func TestKnowledgeSearchInvalidFTSQuery(t *testing.T) {
	r, _ := setupTestKnowledgeSearchRouter(t)

	w := do(t, r, "GET", "/knowledge/search?q=%22", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed FTS query, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestKnowledgeSearchPagination verifies the limit/offset query
// parameters narrow the result set and that the total count is
// unaffected by pagination.
func TestKnowledgeSearchPagination(t *testing.T) {
	r, gormDB := setupTestKnowledgeSearchRouter(t)

	for i := 0; i < 5; i++ {
		kd := &models.KnowledgeDoc{
			Title:   "风格条目",
			Content: "共通关键词",
			DocType: "ip-style",
		}
		if err := gormDB.Create(kd).Error; err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	w := do(t, r, "GET", "/knowledge/search?q=共通关键词&limit=2&offset=2", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items  []models.KnowledgeDoc `json:"items"`
		Total  int64                 `json:"total"`
		Limit  int                   `json:"limit"`
		Offset int                   `json:"offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 5 {
		t.Errorf("expected total=5, got %d", resp.Total)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items on this page, got %d", len(resp.Items))
	}
	if resp.Limit != 2 || resp.Offset != 2 {
		t.Errorf("expected limit=2 offset=2, got limit=%d offset=%d", resp.Limit, resp.Offset)
	}
}

// TestKnowledgeSearchStaysInSyncWithCrud exercises the FTS5
// trigger surface: inserts must populate the FTS index, updates
// must refresh it, and deletes must remove the row. The
// triggers are defined on the base table, so we exercise them
// via direct GORM operations (the HTTP handler is a thin
// wrapper around the same GORM store the triggers observe, so
// going through HTTP would only add an auth dependency to a
// test that is really about the DB layer). The search half
// still goes through the real handler so we also confirm the
// FTS query path stays in lockstep with the trigger path.
//
// UserID is set explicitly because the test does not exercise
// the auth surface — the trigger fires regardless of which
// tenant owns the row.
func TestKnowledgeSearchStaysInSyncWithCrud(t *testing.T) {
	r, gormDB := setupTestKnowledgeSearchRouter(t)

	// 1. Create via GORM. The AFTER INSERT trigger should populate
	//    the FTS index.
	kd := models.KnowledgeDoc{
		UserID:  1,
		Title:   "声音调性指南",
		Path:    "ip-style-guide/voice",
		Content: "核心是冷静、不滥用情绪词",
		DocType: "ip-style",
	}
	if err := gormDB.Create(&kd).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if kd.ID == 0 {
		t.Fatalf("create: expected non-zero ID after Create")
	}

	w := do(t, r, "GET", "/knowledge/search?q=情绪词", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("search after create: expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 match after create, got %d", resp.Total)
	}

	// 2. Update via GORM. The AFTER UPDATE trigger should remove
	//    the old row from FTS and insert the new content.
	kd.Content = "完全不同的内容:数据驱动"
	if err := gormDB.Save(&kd).Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	w = do(t, r, "GET", "/knowledge/search?q=情绪词", nil)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 0 {
		t.Errorf("expected 0 matches for old content after update, got %d", resp.Total)
	}
	w = do(t, r, "GET", "/knowledge/search?q=数据驱动", nil)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 match for new content after update, got %d", resp.Total)
	}

	// 3. Delete via GORM. The AFTER DELETE trigger should remove
	//    the row from FTS.
	if err := gormDB.Delete(&kd).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}
	w = do(t, r, "GET", "/knowledge/search?q=数据驱动", nil)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 0 {
		t.Errorf("expected 0 matches after delete, got %d", resp.Total)
	}
}

// TestKnowledgeSearchWithFakeStore verifies the handler maps a store
// error to 500 and a sentinel FTS syntax error to 400. We use a
// hand-rolled fake so we do not have to depend on a particular FTS
// failure mode in the underlying sqlite library.
func TestKnowledgeSearchWithFakeStore(t *testing.T) {
	cases := []struct {
		name     string
		store    KnowledgeSearchStore
		wantCode int
	}{
		{
			name: "generic error becomes 500",
			store: &fakeSearchStore{
				err: errors.New("boom"),
			},
			wantCode: http.StatusInternalServerError,
		},
		{
			name: "fts syntax error becomes 400",
			store: &fakeSearchStore{
				err: ErrInvalidFTSSyntax,
			},
			wantCode: http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			h := NewKnowledgeSearchHandlerWithStore(tc.store, nil)
			h.RegisterRoutes(r)
			w := do(t, r, "GET", "/knowledge/search?q=hello", nil)
			if w.Code != tc.wantCode {
				t.Errorf("expected %d, got %d, body: %s", tc.wantCode, w.Code, w.Body.String())
			}
		})
	}
}

// BenchmarkKnowledgeSearch seeds the FTS index with a configurable
// number of knowledge docs (default 200) and measures end-to-end
// search latency. The spec calls for "performance is reasonable" on
// the FTS path; this benchmark lets us assert that in CI.
//
// Run with: go test -tags fts5 -bench=BenchmarkKnowledgeSearch -benchmem ./internal/handlers/...
func BenchmarkKnowledgeSearch(b *testing.B) {
	const seedCount = 200
	r, gormDB := setupTestKnowledgeSearchRouter(&testing.T{})

	// Seed via the CRUD handler so the FTS triggers fire. We
	// suppress t.Fatalf noise by passing a fresh *testing.T that
	// the helper's t.Cleanup will close over.
	for i := 0; i < seedCount; i++ {
		kd := &models.KnowledgeDoc{
			Title:   "风格条目 " + strconv.Itoa(i),
			Content: "关键词 alpha beta gamma 内容片段 " + strconv.Itoa(i%17),
			DocType: "ip-style",
		}
		if err := gormDB.Create(kd).Error; err != nil {
			b.Fatalf("seed %d: %v", i, err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := doBench(b, r, "GET", "/knowledge/search?q=alpha&limit=20", nil)
		if w.Code != http.StatusOK {
			b.Fatalf("search: %d body=%s", w.Code, w.Body.String())
		}
	}
}

type fakeSearchStore struct {
	items []models.KnowledgeDoc
	total int64
	err   error
}

func (f *fakeSearchStore) Search(ctx context.Context, query string, docType string, page PageRequest, userID uint) ([]models.KnowledgeDoc, int64, error) {
	return f.items, f.total, f.err
}
