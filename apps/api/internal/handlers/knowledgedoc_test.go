package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/db"
	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// setupTestKnowledgeDocRouter creates an in-memory SQLite DB, runs
// AutoMigrate for KnowledgeDoc, and wires the CRUD routes. It reuses
// newTestDB so the t.Cleanup hook that closes the underlying *sql.DB is
// registered exactly once per test. Each test owns its own gin engine and
// gorm handle so tests stay independent of ordering.
//
// The FTS5 shadow table and its sync triggers are also created here, but
// only when the `fts5` build tag is set. Without the tag, db.EnsureKnowledgeFTS
// is a stub that returns ErrFTS5NotEnabled; the CRUD tests don't need FTS
// to pass, so we ignore that case here and only assert in the FTS-specific
// search tests (see knowledge_search_test.go).
func setupTestKnowledgeDocRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gormDB := newTestDB(t)
	if err := gormDB.AutoMigrate(&models.KnowledgeDoc{}); err != nil {
		t.Fatalf("automigrate KnowledgeDoc: %v", err)
	}
	if err := db.EnsureKnowledgeFTS(gormDB); err != nil && err != db.ErrFTS5NotEnabled {
		t.Fatalf("create FTS surface: %v", err)
	}
	r := gin.New()
	r.Use(middleware.StubUser(1)) // see StubUser rationale
	h := NewKnowledgeDocHandler(gormDB)
	h.RegisterRoutes(r)
	return r, gormDB
}

func TestKnowledgeDocCreate(t *testing.T) {
	r, _ := setupTestKnowledgeDocRouter(t)

	body := map[string]any{
		"title":    "IP 风格指南 - 声音调性",
		"path":     "ip-style-guide/voice",
		"content":  "## 声音调性\n\n核心是冷静、不滥用情绪词...",
		"tags":     "voice,style",
		"doc_type": "ip-style",
	}
	w := do(t, r, "POST", "/knowledge", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var kd models.KnowledgeDoc
	if err := json.Unmarshal(w.Body.Bytes(), &kd); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if kd.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
	if kd.Title != "IP 风格指南 - 声音调性" {
		t.Errorf("unexpected title: %q", kd.Title)
	}
	if kd.Path != "ip-style-guide/voice" {
		t.Errorf("unexpected path: %q", kd.Path)
	}
	if kd.DocType != "ip-style" {
		t.Errorf("unexpected doc_type: %q", kd.DocType)
	}
}

func TestKnowledgeDocList(t *testing.T) {
	r, db := setupTestKnowledgeDocRouter(t)

	// Seed two knowledge docs directly via gorm to keep the test focused on
	// List.
	if err := db.Create(&models.KnowledgeDoc{
		UserID: 1,
		Title:  "D1", Path: "p1", Content: "c1", Tags: "a", DocType: "ip-style",
	}).Error; err != nil {
		t.Fatalf("seed KD1: %v", err)
	}
	if err := db.Create(&models.KnowledgeDoc{
		UserID: 1,
		Title:  "D2", Path: "p2", Content: "c2", Tags: "b", DocType: "sop",
	}).Error; err != nil {
		t.Fatalf("seed KD2: %v", err)
	}

	w := do(t, r, "GET", "/knowledge", nil)
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
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Errorf("expected 2 knowledge docs, got total=%d items=%d", resp.Total, len(resp.Items))
	}
}

// TestKnowledgeDocListFilterByDocType verifies the special filter required
// by the task spec: ?doc_type=ip-style must narrow the list to rows of
// that type only.
func TestKnowledgeDocListFilterByDocType(t *testing.T) {
	r, db := setupTestKnowledgeDocRouter(t)

	seeds := []models.KnowledgeDoc{
		{UserID: 1, Title: "D1", Path: "p1", Content: "c1", DocType: "ip-style"},
		{UserID: 1, Title: "D2", Path: "p2", Content: "c2", DocType: "sop"},
		{UserID: 1, Title: "D3", Path: "p3", Content: "c3", DocType: "ip-style"},
		{UserID: 1, Title: "D4", Path: "p4", Content: "c4", DocType: "prompt"},
	}
	for i := range seeds {
		if err := db.Create(&seeds[i]).Error; err != nil {
			t.Fatalf("seed %s: %v", seeds[i].Title, err)
		}
	}

	w := do(t, r, "GET", "/knowledge?doc_type=ip-style", nil)
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
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Fatalf("expected 2 ip-style docs, got total=%d items=%d", resp.Total, len(resp.Items))
	}
	for _, kd := range resp.Items {
		if kd.DocType != "ip-style" {
			t.Errorf("filter leak: doc_type=%q", kd.DocType)
		}
	}
}

func TestKnowledgeDocGet(t *testing.T) {
	r, db := setupTestKnowledgeDocRouter(t)

	kd := &models.KnowledgeDoc{UserID: 1, Title: "D1", Path: "p1", Content: "c1", DocType: "ip-style"}
	if err := db.Create(kd).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := do(t, r, "GET", "/knowledge/"+strconv.Itoa(int(kd.ID)), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var got models.KnowledgeDoc
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != kd.ID {
		t.Errorf("expected id=%d, got %d", kd.ID, got.ID)
	}
	if got.Title != "D1" {
		t.Errorf("unexpected title: %q", got.Title)
	}
}

func TestKnowledgeDocGetNotFound(t *testing.T) {
	r, _ := setupTestKnowledgeDocRouter(t)

	w := do(t, r, "GET", "/knowledge/9999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeDocUpdate(t *testing.T) {
	r, db := setupTestKnowledgeDocRouter(t)

	kd := &models.KnowledgeDoc{UserID: 1, Title: "D1", Path: "p1", Content: "c1", DocType: "ip-style"}
	if err := db.Create(kd).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	body := map[string]any{
		"title":   "D1-updated",
		"content": "c1-updated",
	}
	w := do(t, r, "PUT", "/knowledge/"+strconv.Itoa(int(kd.ID)), body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var got models.KnowledgeDoc
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "D1-updated" {
		t.Errorf("unexpected title: %q", got.Title)
	}
	if got.Content != "c1-updated" {
		t.Errorf("unexpected content: %q", got.Content)
	}
	// doc_type should be preserved since the request omitted it.
	if got.DocType != "ip-style" {
		t.Errorf("doc_type not preserved: %q", got.DocType)
	}
}

func TestKnowledgeDocUpdateNotFound(t *testing.T) {
	r, _ := setupTestKnowledgeDocRouter(t)

	body := map[string]any{"title": "nope"}
	w := do(t, r, "PUT", "/knowledge/9999", body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeDocDelete(t *testing.T) {
	r, db := setupTestKnowledgeDocRouter(t)

	kd := &models.KnowledgeDoc{UserID: 1, Title: "D1", Path: "p1", Content: "c1", DocType: "ip-style"}
	if err := db.Create(kd).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := do(t, r, "DELETE", "/knowledge/"+strconv.Itoa(int(kd.ID)), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The delete endpoint should return some indicator with the id, e.g.
	// {"deleted": id}. Tolerate both integer and float64 JSON shapes.
	if _, ok := resp["deleted"]; !ok {
		t.Errorf("expected response to include `deleted`, got: %s", w.Body.String())
	}

	// Confirm the row is gone.
	w2 := do(t, r, "GET", "/knowledge/"+strconv.Itoa(int(kd.ID)), nil)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w2.Code)
	}
}

func TestKnowledgeDocDeleteNotFound(t *testing.T) {
	r, _ := setupTestKnowledgeDocRouter(t)

	w := do(t, r, "DELETE", "/knowledge/9999", nil)
	// Returning 404 is the friendliest contract; if the handler returns 200
	// with a not-found body, that is also acceptable. We allow either but
	// require a follow-up GET to be 404.
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Fatalf("expected 200 or 404, got %d, body: %s", w.Code, w.Body.String())
	}
	w2 := do(t, r, "GET", "/knowledge/9999", nil)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 on follow-up GET, got %d", w2.Code)
	}
}

func TestKnowledgeDocInvalidID(t *testing.T) {
	r, _ := setupTestKnowledgeDocRouter(t)

	cases := []struct {
		name, method, path string
	}{
		{"get non-numeric", "GET", "/knowledge/abc"},
		{"get negative", "GET", "/knowledge/-1"},
		{"get zero", "GET", "/knowledge/0"},
		{"update non-numeric", "PUT", "/knowledge/abc"},
		{"delete non-numeric", "DELETE", "/knowledge/abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if tc.method == "PUT" {
				w = do(t, r, tc.method, tc.path, map[string]any{"title": "x"})
			} else {
				w = do(t, r, tc.method, tc.path, nil)
			}
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d, body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestKnowledgeDocCreateInvalidJSON(t *testing.T) {
	r, _ := setupTestKnowledgeDocRouter(t)

	req, _ := http.NewRequest("POST", "/knowledge", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
}
