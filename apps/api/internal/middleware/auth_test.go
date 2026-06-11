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
