package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// newImportExportTestDB returns an in-memory SQLite handle that has
// every collection migrated. We share one DB across the four entity
// tables so the "all" export and cross-entity import tests can read
// from a realistic shape. The :memory: shared cache is used so the
// connection pool can reach a single backing instance.
func newImportExportTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	for _, m := range []any{
		&models.Topic{},
		&models.Script{},
		&models.ContentItem{},
		&models.KnowledgeDoc{},
	} {
		if err := gormDB.AutoMigrate(m); err != nil {
			t.Fatalf("automigrate %T: %v", m, err)
		}
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gormDB
}

func newImportExportRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	gormDB := newImportExportTestDB(t)
	r := gin.New()
	h := NewImportExportHandler(gormDB, nil)
	h.RegisterRoutes(r)
	return r, gormDB
}

// seedFour seeds one row into each of the four collections. Returns a
// summary the test can use to assert on the envelope shape.
func seedFour(t *testing.T, db *gorm.DB) {
	t.Helper()
	topic := models.Topic{Title: "测试选题", Platform: "抖音", Status: "想法"}
	if err := db.Create(&topic).Error; err != nil {
		t.Fatalf("seed topic: %v", err)
	}
	script := models.Script{TopicID: topic.ID, Title: "测试脚本", Content: "正文", Platform: "抖音"}
	if err := db.Create(&script).Error; err != nil {
		t.Fatalf("seed script: %v", err)
	}
	now := time.Now().UTC()
	ci := models.ContentItem{ScriptID: script.ID, Platform: "抖音", PlatformURL: "https://example.com/v/1", PublishedAt: &now}
	if err := db.Create(&ci).Error; err != nil {
		t.Fatalf("seed content item: %v", err)
	}
	kd := models.KnowledgeDoc{Title: "SOP", Path: "sop/voice", Content: "...", DocType: "SOP"}
	if err := db.Create(&kd).Error; err != nil {
		t.Fatalf("seed knowledge: %v", err)
	}
}

func doImportExport(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestExportTopic(t *testing.T) {
	r, db := newImportExportRouter(t)
	seedFour(t, db)

	w := doImportExport(t, r, http.MethodGet, "/export?type=topic", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "opc-export-") {
		t.Errorf("missing attachment header: %q", cd)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected JSON content type, got %q", ct)
	}

	var env ExportEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Version != exportVersion {
		t.Errorf("expected version %d, got %d", exportVersion, env.Version)
	}
	if env.Type != exportTypeTopic {
		t.Errorf("expected type %q, got %q", exportTypeTopic, env.Type)
	}
	if env.ExportedAt.IsZero() {
		t.Errorf("exported_at should be set")
	}
	if len(env.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(env.Items))
	}
	raw, err := json.Marshal(env.Items[0])
	if err != nil {
		t.Fatalf("marshal item: %v", err)
	}
	var topic models.Topic
	if err := json.Unmarshal(raw, &topic); err != nil {
		t.Fatalf("unmarshal topic: %v", err)
	}
	if topic.Title != "测试选题" || topic.Platform != "抖音" {
		t.Errorf("seed roundtrip lost data: %+v", topic)
	}
}

func TestExportScript(t *testing.T) {
	r, db := newImportExportRouter(t)
	seedFour(t, db)

	w := doImportExport(t, r, http.MethodGet, "/export?type=script", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env ExportEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Type != exportTypeScript {
		t.Errorf("expected type=script, got %q", env.Type)
	}
	if len(env.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(env.Items))
	}
}

func TestExportContentItem(t *testing.T) {
	r, db := newImportExportRouter(t)
	seedFour(t, db)

	w := doImportExport(t, r, http.MethodGet, "/export?type=content_item", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env ExportEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Type != exportTypeContent {
		t.Errorf("expected type=content_item, got %q", env.Type)
	}
	if len(env.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(env.Items))
	}
}

func TestExportKnowledge(t *testing.T) {
	r, db := newImportExportRouter(t)
	seedFour(t, db)

	w := doImportExport(t, r, http.MethodGet, "/export?type=knowledge", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env ExportEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Type != exportTypeKnowledge {
		t.Errorf("expected type=knowledge, got %q", env.Type)
	}
	if len(env.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(env.Items))
	}
}

func TestExportAll(t *testing.T) {
	r, db := newImportExportRouter(t)
	seedFour(t, db)

	w := doImportExport(t, r, http.MethodGet, "/export?type=all", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env ExportEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Type != exportTypeAll {
		t.Errorf("expected type=all, got %q", env.Type)
	}
	if len(env.Items) != 4 {
		t.Fatalf("expected 4 items (one per entity), got %d", len(env.Items))
	}
}

func TestExportRejectsUnknownType(t *testing.T) {
	r, _ := newImportExportRouter(t)

	w := doImportExport(t, r, http.MethodGet, "/export?type=bogus", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "type must be one of") {
		t.Errorf("missing guidance: %s", w.Body.String())
	}
}

func TestExportEmptyDB(t *testing.T) {
	r, _ := newImportExportRouter(t)

	w := doImportExport(t, r, http.MethodGet, "/export?type=topic", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env ExportEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(env.Items))
	}
}

func TestImportTopicRoundtrip(t *testing.T) {
	r, _ := newImportExportRouter(t)

	body := importEnvelope{
		Version: exportVersion,
		Type:    exportTypeTopic,
		Items: []json.RawMessage{
			mustRaw(t, models.Topic{Title: "导入 1", Platform: "小红书", Status: "想法"}),
			mustRaw(t, models.Topic{Title: "导入 2", Platform: "哔哩哔哩", Status: "评估"}),
		},
	}
	w := doImportExport(t, r, http.MethodPost, "/import?type=topic", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var res importResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Imported != 2 {
		t.Errorf("expected 2 imported, got %d", res.Imported)
	}
	if len(res.Errors) != 0 {
		t.Errorf("expected no errors, got %+v", res.Errors)
	}
}

func TestImportScript(t *testing.T) {
	r, _ := newImportExportRouter(t)

	body := importEnvelope{
		Version: exportVersion,
		Type:    exportTypeScript,
		Items: []json.RawMessage{
			mustRaw(t, models.Script{TopicID: 1, Title: "脚本 A", Content: "...", Platform: "抖音"}),
		},
	}
	w := doImportExport(t, r, http.MethodPost, "/import?type=script", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var res importResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Imported != 1 {
		t.Errorf("expected 1 imported, got %d", res.Imported)
	}
}

func TestImportContentItem(t *testing.T) {
	r, _ := newImportExportRouter(t)

	body := importEnvelope{
		Version: exportVersion,
		Type:    exportTypeContent,
		Items: []json.RawMessage{
			mustRaw(t, models.ContentItem{ScriptID: 1, Platform: "抖音", PlatformURL: "https://example.com/v/1"}),
		},
	}
	w := doImportExport(t, r, http.MethodPost, "/import?type=content_item", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var res importResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Imported != 1 {
		t.Errorf("expected 1 imported, got %d", res.Imported)
	}
}

func TestImportKnowledge(t *testing.T) {
	r, _ := newImportExportRouter(t)

	body := importEnvelope{
		Version: exportVersion,
		Type:    exportTypeKnowledge,
		Items: []json.RawMessage{
			mustRaw(t, models.KnowledgeDoc{Title: "SOP", Path: "sop/voice", Content: "...", DocType: "SOP"}),
		},
	}
	w := doImportExport(t, r, http.MethodPost, "/import?type=knowledge", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var res importResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Imported != 1 {
		t.Errorf("expected 1 imported, got %d (errors=%+v)", res.Imported, res.Errors)
	}
}

func TestImportRequiresType(t *testing.T) {
	r, _ := newImportExportRouter(t)

	body := importEnvelope{Items: []json.RawMessage{mustRaw(t, models.Topic{Title: "x", Platform: "抖音", Status: "想法"})}}
	w := doImportExport(t, r, http.MethodPost, "/import", body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestImportRejectsUnknownType(t *testing.T) {
	r, _ := newImportExportRouter(t)

	body := importEnvelope{Items: []json.RawMessage{mustRaw(t, models.Topic{Title: "x", Platform: "抖音", Status: "想法"})}}
	w := doImportExport(t, r, http.MethodPost, "/import?type=bogus", body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestImportRejectsEmptyItems(t *testing.T) {
	r, _ := newImportExportRouter(t)

	body := importEnvelope{Type: exportTypeTopic, Items: []json.RawMessage{}}
	w := doImportExport(t, r, http.MethodPost, "/import?type=topic", body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestImportRejectsBadJSON(t *testing.T) {
	r, _ := newImportExportRouter(t)

	req, _ := http.NewRequest(http.MethodPost, "/import?type=topic", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestImportRejectsUnknownVersion(t *testing.T) {
	r, _ := newImportExportRouter(t)

	body := map[string]any{
		"version": 999,
		"type":    exportTypeTopic,
		"items":   []any{map[string]any{"title": "x", "platform": "抖音", "status": "想法"}},
	}
	w := doImportExport(t, r, http.MethodPost, "/import?type=topic", body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "version") {
		t.Errorf("expected version error, got: %s", w.Body.String())
	}
}

// TestImportReportsRowErrors proves that a single bad row in the
// payload is reported in the response, not raised as a 4xx. The
// caller should see partial success.
func TestImportReportsRowErrors(t *testing.T) {
	r, _ := newImportExportRouter(t)

	good := mustRaw(t, models.Topic{Title: "好", Platform: "抖音", Status: "想法"})
	bad := json.RawMessage(`{"title":"","platform":"","status":""}`)
	body := importEnvelope{Type: exportTypeTopic, Items: []json.RawMessage{good, bad}}
	w := doImportExport(t, r, http.MethodPost, "/import?type=topic", body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var res importResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Imported != 1 {
		t.Errorf("expected 1 imported, got %d", res.Imported)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("expected 1 error, got %+v", res.Errors)
	}
	if res.Errors[0].Index != 1 {
		t.Errorf("expected error at index 1, got %d", res.Errors[0].Index)
	}
	if res.Errors[0].Message == "" {
		t.Errorf("expected non-empty error message")
	}
}

// TestImportResetsID asserts that re-importing an exported file does
// not collide on the primary key. GORM would otherwise reject the
// insert with a UNIQUE constraint violation.
func TestImportResetsID(t *testing.T) {
	r, db := newImportExportRouter(t)
	seedFour(t, db)

	// Export the existing topic.
	w := doImportExport(t, r, http.MethodGet, "/export?type=topic", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("export: %d", w.Code)
	}
	// Re-import the same body on the same DB.
	w = doImportExport(t, r, http.MethodPost, "/import?type=topic", json.RawMessage(w.Body.Bytes()))
	if w.Code != http.StatusOK {
		t.Fatalf("re-import: %d body=%s", w.Code, w.Body.String())
	}
	var res importResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.Imported != 1 {
		t.Errorf("expected 1 imported, got %d errors=%+v", res.Imported, res.Errors)
	}
	// Confirm there are now 2 topic rows in the DB.
	var count int64
	if err := db.Model(&models.Topic{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 topics after re-import, got %d", count)
	}
}

func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestExportDateInFilename guards the user-facing filename shape
// (date only, no time) so the team's local scripts that depend on
// "opc-export-YYYY-MM-DD.json" do not break.
func TestExportDateInFilename(t *testing.T) {
	r, _ := newImportExportRouter(t)

	w := doImportExport(t, r, http.MethodGet, "/export?type=topic", nil)
	cd := w.Header().Get("Content-Disposition")
	want := fmt.Sprintf(`opc-export-%s.json`, time.Now().UTC().Format("2006-01-02"))
	if !strings.Contains(cd, want) {
		t.Errorf("expected filename %q in header, got %q", want, cd)
	}
}
