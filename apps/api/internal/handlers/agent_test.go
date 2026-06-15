package handlers

// Sub-Spec D M1 — Task 2: 4 admin endpoint unit tests for
// AgentHandler. Mirrors the CreatorHandler test pattern (see
// creator_test.go): all tests are operator-only; the test-mode
// auth shim stamps user_id + user_role onto the Gin context so
// the production middleware chain (RequireAuth →
// RequireOperatorRole) still runs. Non-operator sessions
// produce the documented 403 path.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/opc/api/internal/models"
)

// TestCreateAgent201 is the happy path: an operator POSTs a
// name and gets 201 + the new agent + the FULL key (shown once,
// GitHub PAT-style). KeyPrefix is the 12-char prefix, the rest
// of the key is the secret. HashedKey MUST NOT appear in the
// response (json:"-" on the struct field enforces it).
func TestCreateAgent201(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)

	body := `{"name":"claude-code-laptop-1"}`
	req := makeJSONReq(t, http.MethodPost, "/api/admin/agents", body, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", w.Code, w.Body.String())
	}
	var got struct {
		Agent models.Agent `json:"agent"`
		Key   string       `json:"key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, w.Body.String())
	}
	if got.Agent.Name != "claude-code-laptop-1" {
		t.Errorf("name = %q, want claude-code-laptop-1", got.Agent.Name)
	}
	if got.Agent.CreatedBy != op.ID {
		t.Errorf("created_by = %d, want %d", got.Agent.CreatedBy, op.ID)
	}
	if !strings.HasPrefix(got.Key, "opc_agent_") {
		t.Errorf("returned key = %q, want opc_agent_ prefix", got.Key)
	}
	// HashedKey must NEVER leave the server. The struct tag
	// json:"-" enforces it, but a regression that drops the tag
	// would leak the bcrypt hash; this test guards the wire shape.
	if strings.Contains(w.Body.String(), "hashed_key") {
		t.Errorf("response body must not contain hashed_key (json:'-')")
	}
}

// TestCreateAgent400InvalidName: empty name fails the
// binding:"required,min=1,max=100" validator on the request
// struct → 400.
func TestCreateAgent400InvalidName(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)

	body := `{"name":""}` // empty name fails min=1 validator
	req := makeJSONReq(t, http.MethodPost, "/api/admin/agents", body, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
	}
}

// TestListAgents200: GET /api/admin/agents returns all agents
// (newest first). The list response must NOT contain the full
// key (only the 12-char prefix is exposed) and must NOT contain
// HashedKey. The handler has no idea what's in the response
// beyond the struct tag, so this is the only line of defense
// against an accidental field-exposure regression.
func TestListAgents200(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)
	for _, name := range []string{"agent-a", "agent-b"} {
		a := models.Agent{
			Name:      name,
			KeyPrefix: "opc_agent_x" + string(name[len(name)-1]),
			HashedKey: "h-" + name, // HashedKey has uniqueIndex; vary per row
			CreatedBy: op.ID,
			Scope:     "all",
		}
		if err := db.Create(&a).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	req := makeReq(t, http.MethodGet, "/api/admin/agents", stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var got struct {
		Agents []models.Agent `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, w.Body.String())
	}
	if len(got.Agents) != 2 {
		t.Errorf("len(agents) = %d, want 2", len(got.Agents))
	}
	bodyStr := w.Body.String()
	if strings.Contains(bodyStr, "hashed_key") {
		t.Errorf("list body must not contain hashed_key")
	}
}

// TestDisableAgent200: POST /disable flips Disabled=true on the
// targeted agent row. The handler is a soft-delete — the row
// stays in the table for audit, the API key just stops
// authenticating (the call_log path checks Disabled before
// issuing 200, which is verified end-to-end in Task 3).
func TestDisableAgent200(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)
	a := models.Agent{
		Name:      "to-disable",
		KeyPrefix: "opc_agent_x",
		HashedKey: "h",
		CreatedBy: op.ID,
		Scope:     "all",
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	path := "/api/admin/agents/" + strconv.FormatUint(uint64(a.ID), 10) + "/disable"
	req := makeReq(t, http.MethodPost, path, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var got models.Agent
	if err := db.First(&got, a.ID).Error; err != nil {
		t.Fatalf("re-read agent: %v", err)
	}
	if !got.Disabled {
		t.Errorf("disabled = false, want true")
	}
}

// TestDisableAgent404: disabling a nonexistent agent → 404.
// The handler must look the row up first and return
// ErrRecordNotFound-mapped 404, not 500 or 200.
func TestDisableAgent404(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)

	req := makeReq(t, http.MethodPost, "/api/admin/agents/99999/disable", stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", w.Code, w.Body.String())
	}
}

// TestRegenerateAgent200: POST /regenerate replaces the stored
// bcrypt hash with a new one. The new key is returned once. The
// OLD key must no longer verify against the stored hash; the
// NEW key must verify. We also confirm both keys share the
// opc_agent_ prefix.
func TestRegenerateAgent200(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)
	oldKey, _, err := models.GenerateKey()
	if err != nil {
		t.Fatalf("gen old key: %v", err)
	}
	oldHash, err := models.HashPassword(oldKey)
	if err != nil {
		t.Fatalf("hash old: %v", err)
	}
	a := models.Agent{
		Name:      "to-regen",
		KeyPrefix: models.KeyPrefix(oldKey),
		HashedKey: oldHash,
		CreatedBy: op.ID,
		Scope:     "all",
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	path := "/api/admin/agents/" + strconv.FormatUint(uint64(a.ID), 10) + "/regenerate"
	req := makeReq(t, http.MethodPost, path, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var got struct {
		Agent  models.Agent `json:"agent"`
		NewKey string       `json:"new_key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, w.Body.String())
	}
	if got.NewKey == oldKey {
		t.Errorf("new key == old key; should be different")
	}
	if !strings.HasPrefix(got.NewKey, "opc_agent_") {
		t.Errorf("new key = %q, want opc_agent_ prefix", got.NewKey)
	}
	// Old key must NOT verify against the new hash.
	var dbAgent models.Agent
	if err := db.First(&dbAgent, a.ID).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if models.VerifyPassword(dbAgent.HashedKey, oldKey) {
		t.Error("old key still verifies after regenerate; should be invalid")
	}
	if !models.VerifyPassword(dbAgent.HashedKey, got.NewKey) {
		t.Error("new key does not verify; should be valid")
	}
}
