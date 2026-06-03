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

// setupTestContentItemRouter creates an in-memory SQLite DB, runs
// AutoMigrate for ContentItem, and wires the CRUD routes. It reuses
// newTestDB so the t.Cleanup hook that closes the underlying *sql.DB is
// registered exactly once per test. We don't depend on the Topic/Script
// migrated DB produced by the topic/script tests because each test owns
// its own gin engine and gorm handle.
func setupTestContentItemRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gormDB := newTestDB(t)
	// AutoMigrate the ContentItem table on this handle. (newTestDB already
	// migrated Topic, so we add ContentItem to keep tests independent of
	// test ordering.)
	if err := gormDB.AutoMigrate(&models.ContentItem{}); err != nil {
		t.Fatalf("automigrate ContentItem: %v", err)
	}
	r := gin.New()
	h := NewContentItemHandler(gormDB)
	h.RegisterRoutes(r)
	return r, gormDB
}

func TestContentItemCreate(t *testing.T) {
	r, _ := setupTestContentItemRouter(t)

	body := map[string]any{
		"script_id":           1,
		"platform":            "抖音",
		"platform_url":        "https://www.douyin.com/video/123",
		"performance_metrics": `{"views": 1000, "likes": 50}`,
	}
	w := do(t, r, "POST", "/content-items", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var ci models.ContentItem
	if err := json.Unmarshal(w.Body.Bytes(), &ci); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ci.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
	if ci.ScriptID != 1 {
		t.Errorf("unexpected script_id: %d", ci.ScriptID)
	}
	if ci.Platform != "抖音" {
		t.Errorf("unexpected platform: %q", ci.Platform)
	}
	if ci.PlatformURL != "https://www.douyin.com/video/123" {
		t.Errorf("unexpected platform_url: %q", ci.PlatformURL)
	}
	if ci.PerformanceMetrics != `{"views": 1000, "likes": 50}` {
		t.Errorf("unexpected performance_metrics: %q", ci.PerformanceMetrics)
	}
}

func TestContentItemList(t *testing.T) {
	r, db := setupTestContentItemRouter(t)

	// Seed two content items directly via gorm to keep the test focused on
	// List.
	if err := db.Create(&models.ContentItem{ScriptID: 1, Platform: "抖音", PlatformURL: "u1"}).Error; err != nil {
		t.Fatalf("seed CI1: %v", err)
	}
	if err := db.Create(&models.ContentItem{ScriptID: 1, Platform: "哔哩哔哩", PlatformURL: "u2"}).Error; err != nil {
		t.Fatalf("seed CI2: %v", err)
	}

	w := do(t, r, "GET", "/content-items", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []models.ContentItem `json:"items"`
		Total int64                `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Errorf("expected 2 content items, got total=%d items=%d", resp.Total, len(resp.Items))
	}
}

func TestContentItemGet(t *testing.T) {
	r, db := setupTestContentItemRouter(t)

	ci := &models.ContentItem{ScriptID: 1, Platform: "抖音", PlatformURL: "u1"}
	if err := db.Create(ci).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := do(t, r, "GET", "/content-items/"+strconv.Itoa(int(ci.ID)), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var got models.ContentItem
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != ci.ID {
		t.Errorf("expected id=%d, got %d", ci.ID, got.ID)
	}
	if got.Platform != "抖音" {
		t.Errorf("unexpected platform: %q", got.Platform)
	}
}

func TestContentItemGetNotFound(t *testing.T) {
	r, _ := setupTestContentItemRouter(t)

	w := do(t, r, "GET", "/content-items/9999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestContentItemUpdate(t *testing.T) {
	r, db := setupTestContentItemRouter(t)

	ci := &models.ContentItem{ScriptID: 1, Platform: "抖音", PlatformURL: "u1"}
	if err := db.Create(ci).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	body := map[string]any{
		"platform_url":        "u1-updated",
		"performance_metrics": `{"views": 2000, "likes": 100}`,
	}
	w := do(t, r, "PUT", "/content-items/"+strconv.Itoa(int(ci.ID)), body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var got models.ContentItem
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PlatformURL != "u1-updated" {
		t.Errorf("unexpected platform_url: %q", got.PlatformURL)
	}
	if got.PerformanceMetrics != `{"views": 2000, "likes": 100}` {
		t.Errorf("unexpected performance_metrics: %q", got.PerformanceMetrics)
	}
	// Platform should be preserved since the request omitted it.
	if got.Platform != "抖音" {
		t.Errorf("platform not preserved: %q", got.Platform)
	}
}

func TestContentItemUpdateNotFound(t *testing.T) {
	r, _ := setupTestContentItemRouter(t)

	body := map[string]any{"platform_url": "nope"}
	w := do(t, r, "PUT", "/content-items/9999", body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestContentItemDelete(t *testing.T) {
	r, db := setupTestContentItemRouter(t)

	ci := &models.ContentItem{ScriptID: 1, Platform: "抖音", PlatformURL: "u1"}
	if err := db.Create(ci).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := do(t, r, "DELETE", "/content-items/"+strconv.Itoa(int(ci.ID)), nil)
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
	w2 := do(t, r, "GET", "/content-items/"+strconv.Itoa(int(ci.ID)), nil)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w2.Code)
	}
}

func TestContentItemDeleteNotFound(t *testing.T) {
	r, _ := setupTestContentItemRouter(t)

	w := do(t, r, "DELETE", "/content-items/9999", nil)
	// Returning 404 is the friendliest contract; if the handler returns 200
	// with a not-found body, that is also acceptable. We allow either but
	// require a follow-up GET to be 404.
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Fatalf("expected 200 or 404, got %d, body: %s", w.Code, w.Body.String())
	}
	w2 := do(t, r, "GET", "/content-items/9999", nil)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 on follow-up GET, got %d", w2.Code)
	}
}

func TestContentItemInvalidID(t *testing.T) {
	r, _ := setupTestContentItemRouter(t)

	cases := []struct {
		name, method, path string
	}{
		{"get non-numeric", "GET", "/content-items/abc"},
		{"get negative", "GET", "/content-items/-1"},
		{"get zero", "GET", "/content-items/0"},
		{"update non-numeric", "PUT", "/content-items/abc"},
		{"delete non-numeric", "DELETE", "/content-items/abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if tc.method == "PUT" {
				w = do(t, r, tc.method, tc.path, map[string]any{"platform_url": "x"})
			} else {
				w = do(t, r, tc.method, tc.path, nil)
			}
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d, body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestContentItemCreateInvalidJSON(t *testing.T) {
	r, _ := setupTestContentItemRouter(t)

	req, _ := http.NewRequest("POST", "/content-items", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
}
