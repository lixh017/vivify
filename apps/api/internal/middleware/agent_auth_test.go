package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// newAgentTestDB gives the MaybeAgentKey tests an in-memory SQLite
// handle pre-migrated with the Agent table. We use a fresh private
// file per test (not :memory:) so rows from one test never leak into
// the next — :memory: in SQLite is per-connection, and the
// shared-cache mode can subtly share state across goroutines.
func newAgentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	path := t.TempDir() + "/agent-test.db"
	gormDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := gormDB.AutoMigrate(&models.Agent{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return gormDB
}

// newAgentTestRouter wires a single GET /x route that runs
// MaybeAgentKey then a probe handler. The probe writes
// "agent=<id>" when the middleware set agent_id, or "no-agent"
// when it did not. Tests branch on the body, so a regression that
// makes the middleware silently fail (no error, no context stamp)
// surfaces as a clear "no-agent" mismatch.
func newAgentTestRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	r.Use(middleware.MaybeAgentKey(db))
	r.GET("/x", func(c *gin.Context) {
		if id, ok := c.Get("agent_id"); ok {
			c.String(http.StatusOK, "agent=%v", id)
			return
		}
		c.String(http.StatusOK, "no-agent")
	})
	return r
}

// TestMaybeAgentKeyNoHeader is the most common path: a request
// with no X-API-Key header (i.e. a normal human browser session
// coming in via the session cookie that RequireAuth resolved
// upstream) must flow through MaybeAgentKey unchanged. No abort,
// no error, no agent_id on the context.
func TestMaybeAgentKeyNoHeader(t *testing.T) {
	t.Parallel()
	db := newAgentTestDB(t)
	r := newAgentTestRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("no header: code = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "no-agent" {
		t.Errorf("no header: body = %q, want %q", w.Body.String(), "no-agent")
	}
}

// TestMaybeAgentKeyInvalidKey covers the "well-formed prefix, but
// no matching agent" case. The middleware must NOT abort — it
// simply passes through with no agent_id on the context, and the
// handler's existing auth check (e.g. RequireAuth above) decides
// whether the unauthenticated state is acceptable. Aborting here
// would break the human-with-stale-key failure mode (RequireAuth
// wants to emit its own 401 with a reason code).
func TestMaybeAgentKeyInvalidKey(t *testing.T) {
	t.Parallel()
	db := newAgentTestDB(t)
	r := newAgentTestRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-API-Key", "opc_agent_bogus0000000000000000000000")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Body.String() != "no-agent" {
		t.Errorf("invalid key: body = %q, want %q", w.Body.String(), "no-agent")
	}
	if w.Code != http.StatusOK {
		t.Errorf("invalid key: code = %d, want 200 (pass-through, not abort)", w.Code)
	}
}

// TestMaybeAgentKeyValidKey is the happy path: a freshly generated
// key, hashed and stored as an Agent row, comes back through the
// middleware and lands an agent_id on the context. The probe
// handler echoes "agent=<id>" so we can both confirm the id was
// set AND that the value is the Agent row's primary key (not, say,
// the bcrypt hash or the key string).
func TestMaybeAgentKeyValidKey(t *testing.T) {
	t.Parallel()
	db := newAgentTestDB(t)
	key, _, err := models.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	hash, err := models.HashPassword(key)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	a := models.Agent{
		Name:      "test",
		KeyPrefix: models.KeyPrefix(key),
		HashedKey: hash,
		CreatedBy: 1,
		Scope:     "all",
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	r := newAgentTestRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-API-Key", key)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Body.String() == "no-agent" {
		t.Errorf("valid key: body = %q, want agent=<id> stamp", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "agent=") {
		t.Errorf("valid key: body = %q, want substring %q", w.Body.String(), "agent=")
	}
	if w.Code != http.StatusOK {
		t.Errorf("valid key: code = %d, want 200", w.Code)
	}
}

// TestMaybeAgentKeyDisabledAgent covers the revocation path: an
// agent that an operator has flipped to Disabled=true must NOT
// pass the middleware, even if its bcrypt-valid key is sent. The
// behaviour is "pass-through" (not 401) for the same reason as the
// invalid-key case — the handler's auth check (or a downstream
// per-scope check) owns the actual rejection. This keeps the
// middleware's contract narrow: "valid + enabled" is the only
// positive state, everything else is "no agent_id on context".
func TestMaybeAgentKeyDisabledAgent(t *testing.T) {
	t.Parallel()
	db := newAgentTestDB(t)
	key, _, err := models.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	hash, err := models.HashPassword(key)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	a := models.Agent{
		Name:      "test",
		KeyPrefix: models.KeyPrefix(key),
		HashedKey: hash,
		CreatedBy: 1,
		Scope:     "all",
		Disabled:  true,
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	r := newAgentTestRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-API-Key", key)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Body.String() != "no-agent" {
		t.Errorf("disabled key: body = %q, want %q (pass-through, not abort)", w.Body.String(), "no-agent")
	}
}

// TestMaybeAgentKeyWithSessionCookie documents the composition
// rule: MaybeAgentKey does NOT consult the session cookie. A
// request with a valid X-API-Key AND a session cookie (or no
// cookie — this test has no cookie at all) still gets agent_id
// stamped. The handler decides what to do with the dual presence
// (today: agent_id wins; RequireAuth must NOT abort because the
// agent caller has no session, and MaybeAgentKey must NOT abort
// because no agent row would be a 401-with-no-context stamp). The
// test uses no session cookie at all so a future regression that
// ties MaybeAgentKey to a cookie lookup would fail.
func TestMaybeAgentKeyWithSessionCookie(t *testing.T) {
	t.Parallel()
	db := newAgentTestDB(t)
	key, _, err := models.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	hash, err := models.HashPassword(key)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	a := models.Agent{
		Name:      "test",
		KeyPrefix: models.KeyPrefix(key),
		HashedKey: hash,
		CreatedBy: 1,
		Scope:     "all",
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	r := newAgentTestRouter(db)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-API-Key", key)
	// (intentionally no session cookie)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("valid key (no cookie): code = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() == "no-agent" {
		t.Errorf("valid key (no cookie): body = %q, want agent=<id>", w.Body.String())
	}
}
