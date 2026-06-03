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

	"github.com/opc/api/internal/models"
)

// setupTestScriptRouter creates an in-memory SQLite DB, runs AutoMigrate for
// Script, and wires the CRUD routes. It reuses newTestDB so the t.Cleanup
// hook that closes the underlying *sql.DB is registered exactly once per
// test. We don't depend on the Topic-migrated DB produced by the topic
// tests, because each test owns its own gin engine and gorm handle.
func setupTestScriptRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gormDB := newTestDB(t)
	// AutoMigrate the Script table on this handle. (newTestDB already migrated
	// Topic, so we add Script to keep tests independent of test ordering.)
	if err := gormDB.AutoMigrate(&models.Script{}); err != nil {
		t.Fatalf("automigrate Script: %v", err)
	}
	r := gin.New()
	h := NewScriptHandler(gormDB)
	h.RegisterRoutes(r)
	return r, gormDB
}

func TestScriptCreate(t *testing.T) {
	r, _ := setupTestScriptRouter(t)

	body := map[string]any{
		"topic_id":   1,
		"title":      "测试脚本",
		"content":    "# 标题\n\n这是脚本内容。",
		"platform":   "抖音",
		"tags":       "a,b,c",
		"word_count": 100,
	}
	w := do(t, r, "POST", "/scripts", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var s models.Script
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
	if s.Title != "测试脚本" {
		t.Errorf("unexpected title: %q", s.Title)
	}
	if s.Content != "# 标题\n\n这是脚本内容。" {
		t.Errorf("unexpected content: %q", s.Content)
	}
	if s.Platform != "抖音" {
		t.Errorf("unexpected platform: %q", s.Platform)
	}
}

func TestScriptList(t *testing.T) {
	r, db := setupTestScriptRouter(t)

	// Seed two scripts directly via gorm to keep the test focused on List.
	if err := db.Create(&models.Script{TopicID: 1, Title: "S1", Content: "c1", Platform: "抖音"}).Error; err != nil {
		t.Fatalf("seed S1: %v", err)
	}
	if err := db.Create(&models.Script{TopicID: 1, Title: "S2", Content: "c2", Platform: "哔哩哔哩"}).Error; err != nil {
		t.Fatalf("seed S2: %v", err)
	}

	w := do(t, r, "GET", "/scripts", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []models.Script `json:"items"`
		Total int64           `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Errorf("expected 2 scripts, got total=%d items=%d", resp.Total, len(resp.Items))
	}
}

func TestScriptGet(t *testing.T) {
	r, db := setupTestScriptRouter(t)

	sc := &models.Script{TopicID: 1, Title: "S1", Content: "c1", Platform: "抖音"}
	if err := db.Create(sc).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := do(t, r, "GET", "/scripts/"+strconv.Itoa(int(sc.ID)), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var got models.Script
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != sc.ID {
		t.Errorf("expected id=%d, got %d", sc.ID, got.ID)
	}
	if got.Title != "S1" {
		t.Errorf("unexpected title: %q", got.Title)
	}
}

func TestScriptGetNotFound(t *testing.T) {
	r, _ := setupTestScriptRouter(t)

	w := do(t, r, "GET", "/scripts/9999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestScriptUpdate(t *testing.T) {
	r, db := setupTestScriptRouter(t)

	sc := &models.Script{TopicID: 1, Title: "S1", Content: "c1", Platform: "抖音"}
	if err := db.Create(sc).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	body := map[string]any{
		"title":      "S1-updated",
		"content":    "c1-updated",
		"word_count": 200,
	}
	w := do(t, r, "PUT", "/scripts/"+strconv.Itoa(int(sc.ID)), body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var got models.Script
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Title != "S1-updated" {
		t.Errorf("unexpected title: %q", got.Title)
	}
	if got.Content != "c1-updated" {
		t.Errorf("unexpected content: %q", got.Content)
	}
	if got.WordCount != 200 {
		t.Errorf("unexpected word_count: %d", got.WordCount)
	}
	// Platform should be preserved since the request omitted it.
	if got.Platform != "抖音" {
		t.Errorf("platform not preserved: %q", got.Platform)
	}
}

func TestScriptUpdateNotFound(t *testing.T) {
	r, _ := setupTestScriptRouter(t)

	body := map[string]any{"title": "nope"}
	w := do(t, r, "PUT", "/scripts/9999", body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestScriptDelete(t *testing.T) {
	r, db := setupTestScriptRouter(t)

	sc := &models.Script{TopicID: 1, Title: "S1", Content: "c1", Platform: "抖音"}
	if err := db.Create(sc).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := do(t, r, "DELETE", "/scripts/"+strconv.Itoa(int(sc.ID)), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The delete endpoint should return some indicator with the id, e.g. {"deleted": id}.
	// Tolerate both integer and float64 JSON shapes.
	if _, ok := resp["deleted"]; !ok {
		t.Errorf("expected response to include `deleted`, got: %s", w.Body.String())
	}

	// Confirm the row is gone.
	w2 := do(t, r, "GET", "/scripts/"+strconv.Itoa(int(sc.ID)), nil)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w2.Code)
	}
}

func TestScriptDeleteNotFound(t *testing.T) {
	r, _ := setupTestScriptRouter(t)

	w := do(t, r, "DELETE", "/scripts/9999", nil)
	// Returning 404 is the friendliest contract; if the handler returns 200
	// with a not-found body, that is also acceptable. We allow either but
	// require a follow-up GET to be 404.
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Fatalf("expected 200 or 404, got %d, body: %s", w.Code, w.Body.String())
	}
	w2 := do(t, r, "GET", "/scripts/9999", nil)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 on follow-up GET, got %d", w2.Code)
	}
}

func TestScriptInvalidID(t *testing.T) {
	r, _ := setupTestScriptRouter(t)

	cases := []struct {
		name, method, path string
	}{
		{"get non-numeric", "GET", "/scripts/abc"},
		{"get negative", "GET", "/scripts/-1"},
		{"get zero", "GET", "/scripts/0"},
		{"update non-numeric", "PUT", "/scripts/abc"},
		{"delete non-numeric", "DELETE", "/scripts/abc"},
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

func TestScriptCreateInvalidJSON(t *testing.T) {
	r, _ := setupTestScriptRouter(t)

	req, _ := http.NewRequest("POST", "/scripts", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
}
