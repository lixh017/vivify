package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// setupTestIPTemplateRouter builds a router with both the knowledge
// CRUD and the IP-template routes wired against the same in-memory
// SQLite DB. We need the knowledge CRUD registered so we can seed test
// data and so the created IP-template docs are visible through the
// existing /knowledge listing.
func setupTestIPTemplateRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gormDB := newTestDB(t)
	// newTestDB only migrates Topic; bring in KnowledgeDoc too because
	// the IP-template table surface is the knowledge_docs table.
	if err := gormDB.AutoMigrate(&models.KnowledgeDoc{}); err != nil {
		t.Fatalf("automigrate KnowledgeDoc: %v", err)
	}
	r := gin.New()
	crud := NewKnowledgeDocHandler(gormDB)
	crud.RegisterRoutes(r)
	ip := NewIPTemplateHandler(gormDB)
	ip.RegisterRoutes(r)
	return r, gormDB
}

// postJSON is a tiny helper to avoid the verbose bytes+json dance for
// the many small request bodies in this file. It mirrors the behavior
// of do() but is purpose-built for the POST-with-JSON case used by the
// IP template tests.
func postJSON(t *testing.T, r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestIPTemplateListEmpty(t *testing.T) {
	r, _ := setupTestIPTemplateRouter(t)

	w := do(t, r, "GET", "/ip-templates", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Templates []IPTemplate `json:"templates"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Templates) != 0 {
		t.Errorf("expected empty templates list, got %d entries", len(resp.Templates))
	}
}

func TestIPTemplateListGroupsByIPType(t *testing.T) {
	r, _ := setupTestIPTemplateRouter(t)

	// Seed three docs under one IP type and two under another. The list
	// endpoint should return exactly two IPTemplate rows with the right
	// counts.
	seed := []map[string]any{
		{"title": "熊猫 - 风格", "path": "ip-style-guide/熊猫/style", "content": "可爱但克制", "doc_type": "ip-style"},
		{"title": "熊猫 - 语气", "path": "ip-style-guide/熊猫/tone", "content": "短句,多用语气词", "doc_type": "ip-style"},
		{"title": "熊猫 - 选题", "path": "ip-style-guide/熊猫/topics", "content": "日常搞笑", "doc_type": "ip-style"},
		{"title": "数字人 - 风格", "path": "ip-style-guide/数字人/style", "content": "科技感", "doc_type": "ip-style"},
		{"title": "数字人 - 语气", "path": "ip-style-guide/数字人/tone", "content": "理性", "doc_type": "ip-style"},
		// An unrelated SOP that should be filtered out of IP templates.
		{"title": "SOP", "path": "sop/release", "content": "release procedure", "doc_type": "sop"},
	}
	for _, doc := range seed {
		w := do(t, r, "POST", "/knowledge", doc)
		if w.Code != http.StatusCreated {
			t.Fatalf("seed %v: expected 201, got %d body=%s", doc, w.Code, w.Body.String())
		}
	}

	w := do(t, r, "GET", "/ip-templates", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Templates []IPTemplate `json:"templates"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Templates) != 2 {
		t.Fatalf("expected 2 templates, got %d: %s", len(resp.Templates), w.Body.String())
	}

	byType := map[string]IPTemplate{}
	for _, tpl := range resp.Templates {
		byType[tpl.Type] = tpl
	}
	if tpl, ok := byType["熊猫"]; !ok {
		t.Errorf("expected 熊猫 template, missing")
	} else if tpl.DocCount != 3 {
		t.Errorf("熊猫: expected doc_count=3, got %d", tpl.DocCount)
	}
	if tpl, ok := byType["数字人"]; !ok {
		t.Errorf("expected 数字人 template, missing")
	} else if tpl.DocCount != 2 {
		t.Errorf("数字人: expected doc_count=2, got %d", tpl.DocCount)
	}
	if _, ok := byType["sop"]; ok {
		t.Errorf("sop should not appear in IP templates list")
	}
}

func TestIPTemplateCreate(t *testing.T) {
	r, _ := setupTestIPTemplateRouter(t)

	resp := postJSON(t, r, "/ip-templates", map[string]any{
		"type":        "古装",
		"name":        "云岚",
		"description": "冷面剑客,反差萌",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d body=%s", resp.Code, resp.Body.String())
	}
	var created IPTemplate
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Type != "古装" {
		t.Errorf("expected type=古装, got %q", created.Type)
	}
	if created.Name != "云岚" {
		t.Errorf("expected name=云岚, got %q", created.Name)
	}
	if created.DocCount != len(ipTemplateStandardDocs) {
		t.Errorf("expected doc_count=%d, got %d", len(ipTemplateStandardDocs), created.DocCount)
	}

	// Verify the 4 standard docs were actually inserted.
	w := do(t, r, "GET", "/knowledge?doc_type=ip-style", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list knowledge: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var listResp struct {
		Items []models.KnowledgeDoc `json:"items"`
		Total int64                 `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listResp.Total != int64(len(ipTemplateStandardDocs)) {
		t.Errorf("expected %d knowledge docs, got %d", len(ipTemplateStandardDocs), listResp.Total)
	}

	// Each path prefix should point at the new IP type.
	seen := map[string]bool{}
	for _, kd := range listResp.Items {
		if kd.Path == "" {
			t.Errorf("empty path: %+v", kd)
			continue
		}
		// We expect paths of the form ip-style-guide/古装/...
		if !startsWith(kd.Path, "ip-style-guide/古装/") {
			t.Errorf("path does not start with ip-style-guide/古装/: %q", kd.Path)
		}
		seen[kd.Path] = true
	}
	for _, tmpl := range ipTemplateStandardDocs {
		expected := "ip-style-guide/古装/" + trimPrefix(tmpl.Path, "ip-style-guide/")
		if !seen[expected] {
			t.Errorf("expected path %q to be created", expected)
		}
	}
}

func TestIPTemplateCreateRejectsMissingFields(t *testing.T) {
	r, _ := setupTestIPTemplateRouter(t)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"empty body", map[string]any{}},
		{"missing type", map[string]any{"name": "x"}},
		{"missing name", map[string]any{"type": "x"}},
		{"blank type", map[string]any{"type": "   ", "name": "x"}},
		{"blank name", map[string]any{"type": "x", "name": "   "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, r, "/ip-templates", tc.body)
			if resp.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d body=%s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestIPTemplateCreateRejectsBadJSON(t *testing.T) {
	r, _ := setupTestIPTemplateRouter(t)
	req, err := http.NewRequest(http.MethodPost, "/ip-templates", bytes.NewReader([]byte("not json")))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestIPTemplateCreateListsBack(t *testing.T) {
	r, _ := setupTestIPTemplateRouter(t)

	resp := postJSON(t, r, "/ip-templates", map[string]any{
		"type": "言情",
		"name": "折枝",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d body=%s", resp.Code, resp.Body.String())
	}

	w := do(t, r, "GET", "/ip-templates", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp2 struct {
		Templates []IPTemplate `json:"templates"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp2.Templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(resp2.Templates))
	}
	if resp2.Templates[0].Type != "言情" {
		t.Errorf("expected type=言情, got %q", resp2.Templates[0].Type)
	}
	if resp2.Templates[0].DocCount != len(ipTemplateStandardDocs) {
		t.Errorf("expected doc_count=%d, got %d", len(ipTemplateStandardDocs), resp2.Templates[0].DocCount)
	}
}

func TestIPTypeFromPath(t *testing.T) {
	cases := []struct {
		path  string
		want  string
		wantOK bool
	}{
		{"ip-style-guide/熊猫/style", "熊猫", true},
		{"ip-style-guide/数字人/tone", "数字人", true},
		{"ip-style-guide/古装/topics", "古装", true},
		{"sop/release", "", false},
		{"ip-style-guide", "", false},
		{"ip-style-guide/", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := ipTypeFromPath(tc.path)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("ipTypeFromPath(%q) = (%q,%v), want (%q,%v)", tc.path, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestDocTitleFromPath(t *testing.T) {
	cases := map[string]string{
		"style":         "风格指南",
		"tone":          "语气调性",
		"topics":        "种子选题",
		"anti-patterns": "反面清单",
		"unknown":       "unknown",
	}
	for in, want := range cases {
		if got := docTitleFromPath(in); got != want {
			t.Errorf("docTitleFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func trimPrefix(s, prefix string) string {
	if startsWith(s, prefix) {
		return s[len(prefix):]
	}
	return s
}
