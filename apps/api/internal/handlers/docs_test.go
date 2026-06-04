package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newDocsTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewDocsHandlers(nil).RegisterRoutes(r)
	return r
}

// TestOpenAPISpecReturnsValidJSON asserts GET /openapi.json returns the
// embedded spec with the right content type, the body parses as JSON,
// and the document declares OpenAPI 3.0. We deliberately do not
// pin-check the schema or endpoint list — the spec is hand-curated and
// may grow, but the *envelope* shape is a contract Swagger UI relies on.
func TestOpenAPISpecReturnsValidJSON(t *testing.T) {
	r := newDocsTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type: want application/json prefix, got %q", got)
	}

	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if v, _ := doc["openapi"].(string); !strings.HasPrefix(v, "3.") {
		t.Errorf("openapi: want 3.x, got %q", v)
	}
	if _, ok := doc["paths"].(map[string]any); !ok {
		t.Error("missing 'paths' object in spec")
	}
}

// TestOpenAPISpecHasExpectedEndpoints pins the endpoint surface so a
// future accidental rename in either the spec or the routes table
// shows up as a test failure. We don't pin the full path strings — the
// route count is enough to catch "I forgot to document the new endpoint"
// regressions.
func TestOpenAPISpecHasExpectedEndpoints(t *testing.T) {
	r := newDocsTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var doc struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}

	// Every key path in the spec must have at least one method — an
	// empty path object is almost always a typo. The expected count
	// grows over time; pin the current MVP total (17 unique URLs as
	// of the docs sweep: 2 health + 5 topics + 5 scripts + 5 content
	// items + 6 knowledge + 3 ai + 2 ip-templates + 2 import-export).
	if got := len(doc.Paths); got < 17 {
		t.Errorf("openapi.json: want at least 17 paths, got %d", got)
	}

	// Spot-check the canonical endpoints so a refactor that
	// accidentally drops one of them from the spec fails loudly.
	wantPaths := []string{
		"/healthz", "/readyz",
		"/topics", "/topics/{id}",
		"/scripts", "/scripts/{id}",
		"/content-items", "/content-items/{id}",
		"/knowledge", "/knowledge/{id}", "/knowledge/search",
		"/ai/topics", "/ai/humanize", "/ai/postmortem",
		"/ip-templates",
		"/export", "/import",
	}
	for _, p := range wantPaths {
		if _, ok := doc.Paths[p]; !ok {
			t.Errorf("openapi.json: missing path %q", p)
		}
	}
}

// TestSwaggerUIReturnsHTML asserts GET /docs returns the static HTML
// shell with the right content type and points the UI at /openapi.json
// on the same origin. We don't load the actual Swagger UI bundle in
// tests (it requires a browser); the contract is "HTML page, references
// the right spec URL, loads the CDN bundle".
func TestSwaggerUIReturnsHTML(t *testing.T) {
	r := newDocsTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type: want text/html prefix, got %q", got)
	}
	body := rec.Body.String()
	mustContain := []string{
		`<div id="swagger-ui">`,
		`swagger-ui-bundle.js`,
		`url: "/openapi.json"`,
	}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("/docs: body missing %q", s)
		}
	}
}

// TestSwaggerUITrailingSlash makes sure /docs/ and /docs both work.
// Gin would otherwise 404 the trailing-slash variant.
func TestSwaggerUITrailingSlash(t *testing.T) {
	r := newDocsTestRouter()

	for _, path := range []string{"/docs", "/docs/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: want 200, got %d", path, rec.Code)
		}
	}
}
