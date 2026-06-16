package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// newCredentialsTestDB returns an in-memory SQLite DB with the
// tables the credential handler touches. We migrate User (FK
// target) + Credential + CallLog so the delete-side audit row
// has somewhere to land. The shared-cache pattern matches the
// auth_test helper so concurrent cases don't fight over a file
// lock.
func newCredentialsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := gormDB.AutoMigrate(
		&models.User{},
		&models.Credential{},
		&models.CallLog{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gormDB
}

// credentialsTestKey is a deterministic 32-byte key used by
// every handler test. We do NOT read it from the env so the
// test outcome is reproducible regardless of the host setup.
var credentialsTestKey = bytes.Repeat([]byte{0x77}, 32)

// setupCredentialsRouter wires the handler with a stub user on
// the Gin context. The middleware package's StubUser is the
// canonical way to inject a user_id without going through a
// real session — see topic_test.go for the same pattern.
func setupCredentialsRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	gormDB := newCredentialsTestDB(t)
	r := gin.New()
	r.Use(middleware.StubUser(1))
	h := NewCredentialHandler(gormDB, credentialsTestKey, nil)
	h.RegisterRoutes(r)
	return r, gormDB
}

// credentialsDo issues an in-memory HTTP request and returns
// the response. Body is JSON-encoded when non-nil. Mirrors the
// helper in topic_test.go so the per-package test idiom stays
// uniform.
func credentialsDo(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
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

// assertNoEncryptedKeyInBody is the regression guard: the
// public wire shape MUST NOT include encrypted_key (in any
// case). We scan the raw bytes — a future regression that
// re-adds the field with a different JSON tag would still
// fail this assertion.
func assertNoEncryptedKeyInBody(t *testing.T, body []byte) {
	t.Helper()
	lower := bytes.ToLower(body)
	if bytes.Contains(lower, []byte("encrypted_key")) {
		t.Fatalf("response leaked encrypted_key field: %s", body)
	}
	if bytes.Contains(lower, []byte("encryptedkey")) {
		t.Fatalf("response leaked encryptedKey field: %s", body)
	}
	if bytes.Contains(lower, []byte("encrypted")) {
		// Catch any "encryptedFoo" variant that might slip past
		// the explicit checks.
		t.Fatalf("response leaked an encrypted* field: %s", body)
	}
}

// idFromJSON extracts a numeric id from a JSON body and
// renders it as a string for URL composition. JSON numbers
// decode to float64, hence the rounding.
func idFromJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	f, ok := m["id"].(float64)
	if !ok {
		t.Fatalf("id missing or non-numeric: %v", m["id"])
	}
	return fmt.Sprintf("%d", uint(f))
}

// TestCredentialsCreateHappyPath covers the canonical POST flow:
// 201, metadata echoed, ciphertext is the result of an AES-GCM
// seal, and the response body never includes the cipher.
func TestCredentialsCreateHappyPath(t *testing.T) {
	r, db := setupCredentialsRouter(t)
	w := credentialsDo(t, r, "POST", "/credentials", map[string]any{
		"provider":      "volcengine",
		"name":          "production-volc",
		"plaintext_key": "AKLT-test-key",
		"scope":         "all",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v; body = %s", err, w.Body.String())
	}
	if got["provider"] != "volcengine" {
		t.Fatalf("provider = %v, want volcengine", got["provider"])
	}
	if got["name"] != "production-volc" {
		t.Fatalf("name = %v, want production-volc", got["name"])
	}
	if _, ok := got["user_id"]; !ok {
		t.Fatalf("response missing user_id: %v", got)
	}
	assertNoEncryptedKeyInBody(t, w.Body.Bytes())

	// The row on disk must have a non-empty EncryptedKey, and
	// the plaintext must not be recoverable by raw byte
	// matching.
	var row models.Credential
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("db.First: %v", err)
	}
	if len(row.EncryptedKey) < 12 {
		t.Fatalf("EncryptedKey on disk is too short: %d", len(row.EncryptedKey))
	}
	if bytes.Contains(row.EncryptedKey, []byte("AKLT-test-key")) {
		t.Fatalf("plaintext leaked into EncryptedKey: %x", row.EncryptedKey)
	}
}

// TestCredentialsCreateValidationErrors covers the negative
// paths the handler must reject: bad provider, missing name,
// missing plaintext_key, empty provider.
func TestCredentialsCreateValidationErrors(t *testing.T) {
	r, _ := setupCredentialsRouter(t)
	cases := []struct {
		name string
		body any
		want int
	}{
		{"bad provider", map[string]any{"provider": "openai", "name": "x", "plaintext_key": "k"}, http.StatusBadRequest},
		{"missing name", map[string]any{"provider": "anthropic", "plaintext_key": "k"}, http.StatusBadRequest},
		{"missing plaintext", map[string]any{"provider": "anthropic", "name": "x"}, http.StatusBadRequest},
		{"empty provider", map[string]any{"provider": "", "name": "x", "plaintext_key": "k"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := credentialsDo(t, r, "POST", "/credentials", tc.body)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d; body = %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

// TestCredentialsList covers GET /credentials. We seed two rows
// and assert the response shape (items, total) and the
// no-cipher guard.
func TestCredentialsList(t *testing.T) {
	r, _ := setupCredentialsRouter(t)
	// Seed
	if w := credentialsDo(t, r, "POST", "/credentials", map[string]any{
		"provider": "volcengine", "name": "a", "plaintext_key": "k1",
	}); w.Code != http.StatusCreated {
		t.Fatalf("seed #1 status = %d, want 201", w.Code)
	}
	if w := credentialsDo(t, r, "POST", "/credentials", map[string]any{
		"provider": "anthropic", "protocol": "anthropic", "base_url": "https://api.anthropic.com", "model_name": "claude-sonnet-4-5", "name": "b", "plaintext_key": "k2",
	}); w.Code != http.StatusCreated {
		t.Fatalf("seed #2 status = %d, want 201", w.Code)
	}

	w := credentialsDo(t, r, "GET", "/credentials", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var got struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Total != 2 || len(got.Items) != 2 {
		t.Fatalf("total/items = %d/%d, want 2/2", got.Total, len(got.Items))
	}
	assertNoEncryptedKeyInBody(t, w.Body.Bytes())
	for _, it := range got.Items {
		if it["name"] == "" || it["provider"] == "" {
			t.Fatalf("missing metadata in list item: %v", it)
		}
	}
}

// TestCredentialsRotate covers PUT /credentials/:id/rotate. We
// re-encrypt with a different plaintext and assert the row's
// EncryptedKey actually changed.
func TestCredentialsRotate(t *testing.T) {
	r, db := setupCredentialsRouter(t)
	// Create
	cw := credentialsDo(t, r, "POST", "/credentials", map[string]any{
		"provider": "anthropic", "protocol": "anthropic", "base_url": "https://api.anthropic.com", "model_name": "claude-sonnet-4-5", "name": "rotate-me", "plaintext_key": "old-key",
	})
	if cw.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", cw.Code)
	}
	idStr := idFromJSON(t, cw.Body.Bytes())

	// Capture the pre-rotate ciphertext.
	var pre models.Credential
	if err := db.First(&pre, 1).Error; err != nil {
		t.Fatalf("db.First pre: %v", err)
	}
	preCipher := append([]byte(nil), pre.EncryptedKey...)

	// Rotate
	w := credentialsDo(t, r, "PUT", "/credentials/"+idStr+"/rotate", map[string]any{
		"plaintext_key": "new-key-2",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("rotate status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	assertNoEncryptedKeyInBody(t, w.Body.Bytes())

	// The on-disk ciphertext must have changed. AES-GCM with a
	// fresh nonce is overwhelmingly likely to produce a
	// different byte sequence even for the same plaintext, but
	// we don't rely on that — we only assert the bytes are not
	// bit-identical.
	var post models.Credential
	if err := db.First(&post, 1).Error; err != nil {
		t.Fatalf("db.First post: %v", err)
	}
	if bytes.Equal(preCipher, post.EncryptedKey) {
		t.Fatalf("rotate did not change the encrypted_key on disk")
	}
}

// TestCredentialsRotateNotFound covers the 404 path. We rotate
// a non-existent id and assert the error shape.
func TestCredentialsRotateNotFound(t *testing.T) {
	r, _ := setupCredentialsRouter(t)
	w := credentialsDo(t, r, "PUT", "/credentials/999/rotate", map[string]any{
		"plaintext_key": "x",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

// TestCredentialsDelete covers DELETE /credentials/:id. We
// assert the row is gone AND the audit row landed in call_logs.
func TestCredentialsDelete(t *testing.T) {
	r, db := setupCredentialsRouter(t)
	cw := credentialsDo(t, r, "POST", "/credentials", map[string]any{
		"provider": "douyin", "name": "del-me", "plaintext_key": "k",
	})
	idStr := idFromJSON(t, cw.Body.Bytes())

	w := credentialsDo(t, r, "DELETE", "/credentials/"+idStr, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204; body = %s", w.Code, w.Body.String())
	}
	// Row gone.
	var row models.Credential
	if err := db.First(&row, 1).Error; err == nil {
		t.Fatalf("row still present after delete: %+v", row)
	}
	// Audit row present.
	var audit models.CallLog
	if err := db.Where("skill = ? AND user_id = ?", "credential_deleted", uint(1)).First(&audit).Error; err != nil {
		t.Fatalf("audit row missing: %v", err)
	}
	if audit.Provider != "douyin" {
		t.Fatalf("audit provider = %q, want douyin", audit.Provider)
	}
}

// TestCredentialsDeleteNotFound is the symmetric 404 path.
func TestCredentialsDeleteNotFound(t *testing.T) {
	r, _ := setupCredentialsRouter(t)
	w := credentialsDo(t, r, "DELETE", "/credentials/12345", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// TestCredentialsNoEncryptedKeyInAnyResponse is the explicit
// regression suite: every endpoint, every status code, every
// response body is scanned for the cipher field. A regression
// that adds the field back is caught here even if the happy-path
// tests don't notice.
func TestCredentialsNoEncryptedKeyInAnyResponse(t *testing.T) {
	r, _ := setupCredentialsRouter(t)
	// Create
	cw := credentialsDo(t, r, "POST", "/credentials", map[string]any{
		"provider": "volcengine", "name": "regress", "plaintext_key": "k",
	})
	assertNoEncryptedKeyInBody(t, cw.Body.Bytes())
	idStr := idFromJSON(t, cw.Body.Bytes())

	// List
	lw := credentialsDo(t, r, "GET", "/credentials", nil)
	assertNoEncryptedKeyInBody(t, lw.Body.Bytes())

	// Rotate
	rw := credentialsDo(t, r, "PUT", "/credentials/"+idStr+"/rotate", map[string]any{
		"plaintext_key": "k2",
	})
	assertNoEncryptedKeyInBody(t, rw.Body.Bytes())

	// Validation-failure body (bad provider).
	bad := credentialsDo(t, r, "POST", "/credentials", map[string]any{
		"provider": "openai", "name": "x", "plaintext_key": "k",
	})
	assertNoEncryptedKeyInBody(t, bad.Body.Bytes())

	// 404 body
	notFound := credentialsDo(t, r, "PUT", "/credentials/999/rotate", map[string]any{
		"plaintext_key": "k",
	})
	assertNoEncryptedKeyInBody(t, notFound.Body.Bytes())
}

// TestCredentialsTenantIsolation is the multi-tenant regression:
// user 2 cannot read, rotate, or delete user 1's credentials.
// We rebuild the router with a different StubUser between
// calls so the multi-tenant guard is exercised against real,
// tenant-isolated queries.
func TestCredentialsTenantIsolation(t *testing.T) {
	gormDB := newCredentialsTestDB(t)
	r := gin.New()
	r.Use(middleware.StubUser(1))
	h := NewCredentialHandler(gormDB, credentialsTestKey, nil)
	h.RegisterRoutes(r)

	// Create as user 1.
	cw := credentialsDo(t, r, "POST", "/credentials", map[string]any{
		"provider": "volcengine", "name": "user-1-row", "plaintext_key": "k",
	})
	if cw.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", cw.Code)
	}
	idStr := idFromJSON(t, cw.Body.Bytes())

	// Now rebuild the router with StubUser(2) — different
	// tenant. The new StubUser overwrites the prior one
	// because the helper simply calls c.Set on every request.
	r2 := gin.New()
	r2.Use(middleware.StubUser(2))
	h2 := NewCredentialHandler(gormDB, credentialsTestKey, nil)
	h2.RegisterRoutes(r2)

	// List as user 2 must be empty.
	lw := credentialsDo(t, r2, "GET", "/credentials", nil)
	if lw.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", lw.Code)
	}
	var listResp struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	_ = json.Unmarshal(lw.Body.Bytes(), &listResp)
	if listResp.Total != 0 {
		t.Fatalf("user 2 sees %d items, want 0", listResp.Total)
	}

	// Rotate as user 2 must 404, not 200.
	rw := credentialsDo(t, r2, "PUT", "/credentials/"+idStr+"/rotate", map[string]any{
		"plaintext_key": "leaked",
	})
	if rw.Code != http.StatusNotFound {
		t.Fatalf("rotate status = %d, want 404; body = %s", rw.Code, rw.Body.String())
	}

	// Delete as user 2 must 404.
	dw := credentialsDo(t, r2, "DELETE", "/credentials/"+idStr, nil)
	if dw.Code != http.StatusNotFound {
		t.Fatalf("delete status = %d, want 404", dw.Code)
	}

	// Confirm the original row is still there and untouched.
	var row models.Credential
	if err := gormDB.First(&row, 1).Error; err != nil {
		t.Fatalf("user 1 row disappeared: %v", err)
	}
	if row.UserID != 1 {
		t.Fatalf("row user_id = %d, want 1", row.UserID)
	}
}

// TestCredential_Create_AcceptsProtocol covers the Phase 4
// user-configurable provider path. POSTing an Anthropic-protocol
// credential with base_url + model_name must return 201 and persist
// all four Phase 4 fields. The resolver (Task 4) reads them back
// unchanged to construct a live AnthropicCompatProvider.
func TestCredential_Create_AcceptsProtocol(t *testing.T) {
	r, db := setupCredentialsRouter(t)
	w := credentialsDo(t, r, "POST", "/credentials", map[string]any{
		"provider":      "anthropic",
		"protocol":      "anthropic",
		"base_url":      "https://api.anthropic.com",
		"model_name":    "claude-haiku-4-5",
		"name":          "primary",
		"plaintext_key": "sk-test-key",
		"scope":         "all",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v; body = %s", err, w.Body.String())
	}
	if got["protocol"] != "anthropic" {
		t.Fatalf("protocol = %v, want anthropic", got["protocol"])
	}
	if got["base_url"] != "https://api.anthropic.com" {
		t.Fatalf("base_url = %v, want https://api.anthropic.com", got["base_url"])
	}
	if got["model_name"] != "claude-haiku-4-5" {
		t.Fatalf("model_name = %v, want claude-haiku-4-5", got["model_name"])
	}
	assertNoEncryptedKeyInBody(t, w.Body.Bytes())

	// Row on disk must carry the Phase 4 fields, normalized to
	// lowercase per Validate. EncryptedKey must be a real AES-GCM
	// seal, not the plaintext.
	var row models.Credential
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("db.First: %v", err)
	}
	if row.Protocol != "anthropic" {
		t.Fatalf("db protocol = %q, want anthropic", row.Protocol)
	}
	if row.BaseURL != "https://api.anthropic.com" {
		t.Fatalf("db base_url = %q, want https://api.anthropic.com", row.BaseURL)
	}
	if row.ModelName != "claude-haiku-4-5" {
		t.Fatalf("db model_name = %q, want claude-haiku-4-5", row.ModelName)
	}
	if len(row.EncryptedKey) < 12 {
		t.Fatalf("EncryptedKey too short: %d bytes", len(row.EncryptedKey))
	}
	if bytes.Contains(row.EncryptedKey, []byte("sk-test-key")) {
		t.Fatalf("plaintext leaked into EncryptedKey: %x", row.EncryptedKey)
	}
}

// TestCredential_Create_RejectsMissingModelName covers the
// Phase 4 Validate rule: for anthropic/openai providers both
// protocol AND model_name are required. A POST with protocol but
// no model_name must return 400 before any row is persisted.
func TestCredential_Create_RejectsMissingModelName(t *testing.T) {
	r, db := setupCredentialsRouter(t)
	w := credentialsDo(t, r, "POST", "/credentials", map[string]any{
		"provider":      "anthropic",
		"protocol":      "anthropic",
		"base_url":      "https://api.anthropic.com",
		"name":          "incomplete",
		"plaintext_key": "sk-test-key",
		"scope":         "all",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	// No row should have been written — Validate ran before
	// the insert path.
	var count int64
	if err := db.Model(&models.Credential{}).Count(&count).Error; err != nil {
		t.Fatalf("db count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after validation failure, got %d", count)
	}
}
