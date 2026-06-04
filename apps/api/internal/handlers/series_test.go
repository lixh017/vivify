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

	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// setupTestSeriesRouter creates an in-memory SQLite DB, runs AutoMigrate for
// Series, and wires the CRUD routes. It reuses newTestDB so the t.Cleanup
// hook that closes the underlying *sql.DB is registered exactly once per
// test.
func setupTestSeriesRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gormDB := newTestDB(t)
	if err := gormDB.AutoMigrate(&models.Series{}); err != nil {
		t.Fatalf("automigrate Series: %v", err)
	}
	r := gin.New()
	r.Use(middleware.StubUser(1)) // see StubUser rationale
	h := NewSeriesHandler(gormDB)
	h.RegisterRoutes(r)
	return r, gormDB
}

func TestSeriesCreate(t *testing.T) {
	r, _ := setupTestSeriesRouter(t)

	body := map[string]any{
		"name":        "熊猫日记",
		"description": "日常治愈系短片系列",
		"ip_id":       1,
	}
	w := do(t, r, "POST", "/series", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	var s models.Series
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.ID == 0 {
		t.Errorf("expected non-zero ID")
	}
	if s.Name != "熊猫日记" {
		t.Errorf("unexpected name: %q", s.Name)
	}
	if s.Description != "日常治愈系短片系列" {
		t.Errorf("unexpected description: %q", s.Description)
	}
	if s.IPID != 1 {
		t.Errorf("unexpected ip_id: %d", s.IPID)
	}
}

func TestSeriesList(t *testing.T) {
	r, db := setupTestSeriesRouter(t)

	// Seed two series directly via gorm to keep the test focused on List.
	if err := db.Create(&models.Series{UserID: 1, Name: "S1", Description: "D1", IPID: 1}).Error; err != nil {
		t.Fatalf("seed S1: %v", err)
	}
	if err := db.Create(&models.Series{UserID: 1, Name: "S2", Description: "D2", IPID: 1}).Error; err != nil {
		t.Fatalf("seed S2: %v", err)
	}

	w := do(t, r, "GET", "/series", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []models.Series `json:"items"`
		Total int64           `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 || len(resp.Items) != 2 {
		t.Errorf("expected 2 series, got total=%d items=%d", resp.Total, len(resp.Items))
	}
}

func TestSeriesGet(t *testing.T) {
	r, db := setupTestSeriesRouter(t)

	s := &models.Series{UserID: 1, Name: "S1", Description: "D1", IPID: 1}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := do(t, r, "GET", "/series/"+strconv.Itoa(int(s.ID)), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var got models.Series
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("expected id=%d, got %d", s.ID, got.ID)
	}
	if got.Name != "S1" {
		t.Errorf("unexpected name: %q", got.Name)
	}
}

func TestSeriesGetNotFound(t *testing.T) {
	r, _ := setupTestSeriesRouter(t)

	w := do(t, r, "GET", "/series/9999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestSeriesUpdate(t *testing.T) {
	r, db := setupTestSeriesRouter(t)

	s := &models.Series{UserID: 1, Name: "S1", Description: "D1", IPID: 1}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	body := map[string]any{
		"name":        "S1-updated",
		"description": "D1-updated",
	}
	w := do(t, r, "PUT", "/series/"+strconv.Itoa(int(s.ID)), body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var got models.Series
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "S1-updated" {
		t.Errorf("unexpected name: %q", got.Name)
	}
	if got.Description != "D1-updated" {
		t.Errorf("unexpected description: %q", got.Description)
	}
	// IPID should be preserved since the request omitted it.
	if got.IPID != 1 {
		t.Errorf("ip_id not preserved: %d", got.IPID)
	}
}

func TestSeriesUpdateNotFound(t *testing.T) {
	r, _ := setupTestSeriesRouter(t)

	body := map[string]any{"name": "nope"}
	w := do(t, r, "PUT", "/series/9999", body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestSeriesDelete(t *testing.T) {
	r, db := setupTestSeriesRouter(t)

	s := &models.Series{UserID: 1, Name: "S1", Description: "D1", IPID: 1}
	if err := db.Create(s).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	w := do(t, r, "DELETE", "/series/"+strconv.Itoa(int(s.ID)), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// The delete endpoint should return some indicator with the id, e.g. {"deleted": id}.
	if _, ok := resp["deleted"]; !ok {
		t.Errorf("expected response to include `deleted`, got: %s", w.Body.String())
	}

	// Confirm the row is gone.
	w2 := do(t, r, "GET", "/series/"+strconv.Itoa(int(s.ID)), nil)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", w2.Code)
	}
}

func TestSeriesDeleteNotFound(t *testing.T) {
	r, _ := setupTestSeriesRouter(t)

	w := do(t, r, "DELETE", "/series/9999", nil)
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Fatalf("expected 200 or 404, got %d, body: %s", w.Code, w.Body.String())
	}
	w2 := do(t, r, "GET", "/series/9999", nil)
	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 on follow-up GET, got %d", w2.Code)
	}
}

func TestSeriesInvalidID(t *testing.T) {
	r, _ := setupTestSeriesRouter(t)

	cases := []struct {
		name, method, path string
	}{
		{"get non-numeric", "GET", "/series/abc"},
		{"get negative", "GET", "/series/-1"},
		{"get zero", "GET", "/series/0"},
		{"update non-numeric", "PUT", "/series/abc"},
		{"delete non-numeric", "DELETE", "/series/abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if tc.method == "PUT" {
				w = do(t, r, tc.method, tc.path, map[string]any{"name": "x"})
			} else {
				w = do(t, r, tc.method, tc.path, nil)
			}
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d, body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSeriesCreateInvalidJSON(t *testing.T) {
	r, _ := setupTestSeriesRouter(t)

	req, _ := http.NewRequest("POST", "/series", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
}
