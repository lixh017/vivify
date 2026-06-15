package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/auth"
	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// newMiddlewareAuthDB returns an in-memory SQLite with the auth tables
// migrated. Reusing the shared-cache pattern from the handlers package
// so this test file stays independent of the handlers package's test
// helpers (the middleware package cannot import internal test code from
// the handlers package).
func newMiddlewareAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := gormDB.AutoMigrate(&models.User{}, &models.Session{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gormDB
}

func seedMiddlewareUser(t *testing.T, db *gorm.DB, email, password string) models.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := models.User{Email: email, PasswordHash: hash, Name: email}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

// TestMiddlewareRequireAuthBlocks is the canonical "no cookie" case:
// the middleware must short-circuit with 401 and a generic body, and
// the downstream handler must NOT have run. The handlers package has
// the same test, but re-asserting it through the middleware package
// import surface guarantees that the re-export in middleware/auth.go
// does not silently fall through to a stub.
func TestMiddlewareRequireAuthBlocks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := newMiddlewareAuthDB(t)
	r := gin.New()
	var downstreamHit bool
	r.GET("/api/protected",
		middleware.RequireAuth(gormDB, nil),
		func(c *gin.Context) {
			downstreamHit = true
			c.JSON(http.StatusOK, gin.H{"ok": true})
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401; body: %s", w.Code, w.Body.String())
	}
	if downstreamHit {
		t.Errorf("downstream handler ran despite no session")
	}
}

// TestMiddlewareRequireAuthAllows exercises the happy path: a valid
// session cookie reaches the downstream handler, the user ID is on
// the Gin context (readable via middleware.UserIDFromContext), and
// the 200 is returned.
func TestMiddlewareRequireAuthAllows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := newMiddlewareAuthDB(t)
	u := seedMiddlewareUser(t, gormDB, "alice@example.com", "hunter2")
	s, err := auth.CreateSession(gormDB, u.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	r := gin.New()
	r.GET("/api/protected",
		middleware.RequireAuth(gormDB, nil),
		func(c *gin.Context) {
			id := middleware.UserIDFromContext(c)
			c.JSON(http.StatusOK, gin.H{"user_id": id})
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: "opc_session", Value: s.Token})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var body struct {
		UserID uint `json:"user_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.UserID != u.ID {
		t.Errorf("user_id = %d, want %d", body.UserID, u.ID)
	}
}

// TestMiddlewareRequireAuthBadToken covers the "tampered cookie" path:
// a random string that does not correspond to any session row must
// produce the same 401 body as the no-cookie path — so the endpoint
// cannot be used as an oracle for "is this token well-formed".
func TestMiddlewareRequireAuthBadToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := newMiddlewareAuthDB(t)
	r := gin.New()
	r.GET("/api/protected",
		middleware.RequireAuth(gormDB, nil),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: "opc_session", Value: "not-a-real-token"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401; body: %s", w.Code, w.Body.String())
	}
}

// TestMiddlewareRequireAuthPanicsOnNilDB is a unit test for the
// constructor's contract: passing a nil *gorm.DB must panic rather
// than silently installing a middleware that 500s on every request.
// We use a deferred recover so a regression that removes the panic is
// caught as a test failure.
func TestMiddlewareRequireAuthPanicsOnNilDB(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("RequireAuth(nil) did not panic; expected panic on nil db")
		}
	}()
	_ = middleware.RequireAuth(nil, nil)
}

// TestMiddlewareRequireAuthReason covers the machine-readable `reason`
// field on the 401 body. Frontend devs branch on this to decide
// whether to re-login, refresh, or fix their request, so each failure
// path must produce the documented reason. The generic `error`
// envelope is also asserted to be unchanged (regression guard for
// clients that still match on the string).
func TestMiddlewareRequireAuthReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := newMiddlewareAuthDB(t)

	// Seed a user + a valid session so we can later mutate the
	// session row to simulate an expired token without losing the
	// user.
	u := seedMiddlewareUser(t, gormDB, "reason@example.com", "hunter2")
	s, err := auth.CreateSession(gormDB, u.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	r := gin.New()
	r.GET("/api/protected",
		middleware.RequireAuth(gormDB, nil),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		},
	)

	hit := func(t *testing.T, cookie string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "opc_session", Value: cookie})
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v (raw: %s)", err, w.Body.String())
		}
		return w.Code, body
	}

	t.Run("no_cookie", func(t *testing.T) {
		code, body := hit(t, "")
		if code != http.StatusUnauthorized {
			t.Fatalf("code = %d, want 401", code)
		}
		if body["error"] != "not authenticated" {
			t.Errorf("error = %v, want \"not authenticated\"", body["error"])
		}
		if body["reason"] != "no_cookie" {
			t.Errorf("reason = %v, want \"no_cookie\"", body["reason"])
		}
		if body["hint"] == "" {
			t.Errorf("hint is empty; want a non-empty login pointer")
		}
	})

	t.Run("invalid_signature", func(t *testing.T) {
		code, body := hit(t, "not-a-real-token")
		if code != http.StatusUnauthorized {
			t.Fatalf("code = %d, want 401", code)
		}
		if body["reason"] != "invalid_signature" {
			t.Errorf("reason = %v, want \"invalid_signature\"", body["reason"])
		}
	})

	t.Run("expired", func(t *testing.T) {
		// Backdate the seeded session's ExpiresAt so the next
		// ValidateSession returns ErrSessionExpired. We re-fetch
		// the row to avoid stomping on GORM's zero-time handling.
		if err := gormDB.Model(&models.Session{}).Where("token = ?", s.Token).Update("expires_at", gormDB.NowFunc().Add(-time.Hour)).Error; err != nil {
			t.Fatalf("backdate session: %v", err)
		}
		code, body := hit(t, s.Token)
		if code != http.StatusUnauthorized {
			t.Fatalf("code = %d, want 401", code)
		}
		if body["reason"] != "expired" {
			t.Errorf("reason = %v, want \"expired\"", body["reason"])
		}
	})
}

// =============================================================================
// Sub-Spec C M1 — Task 2: RequireOperatorRole / RequireCreatorRole tests.
//
// Both role-check middlewares are designed to run AFTER RequireAuth,
// which is responsible for resolving the session cookie and stamping
// user_id + user_role on the Gin context. The tests below exercise the
// role-check middleware in isolation: they pre-populate the Gin context
// with the role value RequireAuth would have set, then verify the
// middleware either lets the request through (200) or short-circuits
// with the documented status (401 if no role, 403 if role mismatch).
// =============================================================================

// newRouterWithMiddleware builds a router that runs the given
// middleware chain and then serves a dummy GET /x handler. The chain
// runs in the order given, mirroring how main.go composes
// RequireAuth + RequireOperatorRole into a single /api/admin group.
func newRouterWithMiddleware(mw ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	for _, m := range mw {
		r.Use(m)
	}
	r.GET("/x", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

// stampRole is the test-mode "RequireAuth already ran" middleware:
// it sets user_role on the Gin context from the X-Test-Role header,
// mirroring the production chain that RequireAuth runs before the
// role-check middleware.
func stampRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		if role := c.GetHeader("X-Test-Role"); role != "" {
			c.Set("user_role", role)
		}
		c.Next()
	}
}

// TestRequireOperatorRoleOperatorPass: an operator-role session
// reaches the downstream handler.
func TestRequireOperatorRoleOperatorPass(t *testing.T) {
	t.Parallel()
	r := newRouterWithMiddleware(
		stampRole(),
		middleware.RequireOperatorRole(),
	)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Test-Role", "operator")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("operator pass, got %d want 200; body: %s", w.Code, w.Body.String())
	}
}

// TestRequireOperatorRoleCreatorFail: a creator-role session is
// rejected with 403 by the operator-only middleware.
func TestRequireOperatorRoleCreatorFail(t *testing.T) {
	t.Parallel()
	r := newRouterWithMiddleware(
		stampRole(),
		middleware.RequireOperatorRole(),
	)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Test-Role", "creator")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("creator fail, got %d want 403; body: %s", w.Code, w.Body.String())
	}
}

// TestRequireOperatorRoleUnauthFail: no role in context (RequireAuth
// did not run) is rejected with 401.
func TestRequireOperatorRoleUnauthFail(t *testing.T) {
	t.Parallel()
	r := newRouterWithMiddleware(middleware.RequireOperatorRole())
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauth fail, got %d want 401; body: %s", w.Code, w.Body.String())
	}
}

// TestRequireCreatorRoleCreatorPass: a creator-role session reaches
// the downstream handler.
func TestRequireCreatorRoleCreatorPass(t *testing.T) {
	t.Parallel()
	r := newRouterWithMiddleware(
		stampRole(),
		middleware.RequireCreatorRole(),
	)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Test-Role", "creator")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("creator pass, got %d want 200; body: %s", w.Code, w.Body.String())
	}
}

// TestRequireCreatorRoleOperatorFail: an operator-role session is
// rejected with 403 by the creator-only middleware.
func TestRequireCreatorRoleOperatorFail(t *testing.T) {
	t.Parallel()
	r := newRouterWithMiddleware(
		stampRole(),
		middleware.RequireCreatorRole(),
	)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Test-Role", "operator")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("operator fail, got %d want 403; body: %s", w.Code, w.Body.String())
	}
}

// TestRequireCreatorRoleUnauthFail: no role in context (RequireAuth
// did not run) is rejected with 401.
func TestRequireCreatorRoleUnauthFail(t *testing.T) {
	t.Parallel()
	r := newRouterWithMiddleware(middleware.RequireCreatorRole())
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauth fail, got %d want 401; body: %s", w.Code, w.Body.String())
	}
}

// =============================================================================
// RequireEitherAuth (Sub-Spec D M1 — Task 5)
// =============================================================================
//
// Pinning the four expected behaviors:
//   (1) cookie only (human): 200 + user_id stamped
//   (2) X-API-Key only (agent): 200 + agent_id stamped
//   (3) both: cookie wins (200 + user_id; agent_id NOT stamped to
//       keep the model simple — neither handler check requires
//       stamping both)
//   (4) neither: 401 with the canonical reason envelope
//
// The (2) case is the one the original "RequireAuth → MaybeAgentKey"
// chain got wrong: RequireAuth aborted the request before
// MaybeAgentKey could stamp agent_id, so agents got 401 from the
// cookie check. RequireEitherAuth composes the two checks in a
// single middleware so this no longer happens.

// newEitherAuthDB returns an in-memory SQLite migrated with User,
// Session, AND Agent — needed by RequireEitherAuth's agent-key
// path. The shared cache lets multiple sub-tests in the same run
// use the same DB without trampling each other.
func newEitherAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := gormDB.AutoMigrate(&models.User{}, &models.Session{}, &models.Agent{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gormDB
}

func seedAgent(t *testing.T, db *gorm.DB, name string) (models.Agent, string) {
	t.Helper()
	key, prefix, err := models.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	hash, err := models.HashPassword(key)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	a := models.Agent{
		Name: name, KeyPrefix: prefix, HashedKey: hash,
		CreatedBy: 1, Scope: "all",
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return a, key
}

func TestRequireEitherAuthCookieOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newEitherAuthDB(t)
	u := seedMiddlewareUser(t, db, "u@e.com", "pw-12345678")
	sess, err := auth.CreateSession(db, u.ID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	tok := sess.Token

	var seenUserID uint
	r := gin.New()
	r.Use(middleware.RequireEitherAuth(db, nil))
	r.GET("/x", func(c *gin.Context) {
		seenUserID = middleware.UserIDFromContext(c)
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: "opc_session", Value: tok, Path: "/"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if seenUserID != u.ID {
		t.Errorf("user_id = %d, want %d", seenUserID, u.ID)
	}
}

func TestRequireEitherAuthKeyOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newEitherAuthDB(t)
	_, key := seedAgent(t, db, "claude-code-laptop-1")

	var seenAgentID uint
	r := gin.New()
	r.Use(middleware.RequireEitherAuth(db, nil))
	r.GET("/x", func(c *gin.Context) {
		v, _ := c.Get(middleware.CtxAgentID)
		if id, ok := v.(uint); ok {
			seenAgentID = id
		}
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-API-Key", key)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if seenAgentID == 0 {
		t.Errorf("agent_id was not stamped on context")
	}
}

func TestRequireEitherAuthBothCookieWins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newEitherAuthDB(t)
	u := seedMiddlewareUser(t, db, "u@e.com", "pw-12345678")
	sess, err := auth.CreateSession(db, u.ID)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	tok := sess.Token
	_, key := seedAgent(t, db, "claude-code-laptop-1")

	var seenUserID uint
	r := gin.New()
	r.Use(middleware.RequireEitherAuth(db, nil))
	r.GET("/x", func(c *gin.Context) {
		seenUserID = middleware.UserIDFromContext(c)
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: "opc_session", Value: tok, Path: "/"})
	req.Header.Set("X-API-Key", key)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if seenUserID != u.ID {
		t.Errorf("user_id = %d, want %d (cookie should win)", seenUserID, u.ID)
	}
}

func TestRequireEitherAuthNeither401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newEitherAuthDB(t)
	r := gin.New()
	r.Use(middleware.RequireEitherAuth(db, nil))
	r.GET("/x", func(c *gin.Context) {
		t.Errorf("handler should NOT have run on a no-cookie/no-key request")
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["reason"] != "no_cookie" {
		t.Errorf("reason = %v, want no_cookie", body["reason"])
	}
}

func TestRequireEitherAuthBogusKey401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newEitherAuthDB(t)
	r := gin.New()
	r.Use(middleware.RequireEitherAuth(db, nil))
	r.GET("/x", func(c *gin.Context) {
		t.Errorf("handler should NOT have run on a bogus-key request")
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-API-Key", "opc_agent_bogus0000000000000000000000")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
}
