package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/auth"
	"github.com/opc/api/internal/models"
)

// newAuthTestDB returns an in-memory SQLite DB with the auth-relevant
// tables migrated. Reusing the same shared-cache pattern as the rest
// of the handlers' test helpers so concurrent cases don't fight over
// a file lock.
func newAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		// Production uses TranslateError so the duplicate-email path
		// in Register can rely on gorm.ErrDuplicatedKey instead of
		// matching on the SQLite-specific "UNIQUE constraint failed"
		// English text. Mirror that here so the test surface matches
		// the production surface.
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

// newAuthTestRouter wires an AuthHandler (and the auth surface only).
// RequireAuth is NOT applied — the tests cover the auth surface in
// isolation; the protected surface is exercised by the existing
// per-entity test suites with the auth context supplied manually.
func newAuthTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gormDB := newAuthTestDB(t)
	r := gin.New()
	h := NewAuthHandler(gormDB, nil)
	h.RegisterRoutes(r)
	return r, gormDB
}

// seedUser inserts a fresh user with the given plaintext password.
// Returns the user (with hashed PasswordHash) so callers can assert
// on the ID and email.
func seedUser(t *testing.T, db *gorm.DB, email, password string) models.User {
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

// TestAuthLoginSuccess exercises the happy path: register a user via
// the store, POST /api/auth/login with the right credentials, and
// expect 200 + a Set-Cookie header containing the session token.
func TestAuthLoginSuccess(t *testing.T) {
	r, db := newAuthTestRouter(t)
	seedUser(t, db, "alice@example.com", "hunter2")

	w := doJSON(t, r, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    "alice@example.com",
		"password": "hunter2",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login code = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var body struct {
		User struct {
			ID    uint   `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.User.Email != "alice@example.com" {
		t.Errorf("user email = %q, want alice@example.com", body.User.Email)
	}
	if body.User.ID == 0 {
		t.Errorf("user id is zero")
	}

	// The cookie must be present, HttpOnly, and named opc_session.
	cookies := w.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "opc_session" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatalf("no opc_session cookie in response; got: %v", cookies)
	}
	if !found.HttpOnly {
		t.Errorf("opc_session cookie is not HttpOnly")
	}
	if found.Value == "" {
		t.Errorf("opc_session cookie value is empty")
	}
	if found.Path != "/api" {
		t.Errorf("opc_session cookie path = %q, want /api", found.Path)
	}
}

// TestAuthLoginWrongPassword is the negative case: any deviation
// from the correct password must 401 with a generic body that does
// not distinguish "unknown user" from "wrong password".
func TestAuthLoginWrongPassword(t *testing.T) {
	r, db := newAuthTestRouter(t)
	seedUser(t, db, "bob@example.com", "hunter2")

	w := doJSON(t, r, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    "bob@example.com",
		"password": "wrong",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-password code = %d, want 401; body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("invalid email or password")) {
		t.Errorf("body = %q, want it to mention 'invalid email or password'", body)
	}
}

// TestAuthLoginUnknownEmail must produce the same body / status as
// the wrong-password case so the endpoint cannot be used to
// enumerate valid emails.
func TestAuthLoginUnknownEmail(t *testing.T) {
	r, _ := newAuthTestRouter(t)

	w := doJSON(t, r, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    "ghost@example.com",
		"password": "whatever",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown-email code = %d, want 401; body: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("invalid email or password")) {
		t.Errorf("body = %q, want the generic 'invalid email or password' message", w.Body.String())
	}
}

// TestAuthLoginInvalidBody covers the 400 branch: missing fields,
// malformed JSON, etc.
func TestAuthLoginInvalidBody(t *testing.T) {
	r, _ := newAuthTestRouter(t)

	cases := []struct {
		name string
		body any
	}{
		{"missing password", map[string]any{"email": "x@y"}},
		{"missing email", map[string]any{"password": "x"}},
		{"empty", map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/api/auth/login", tc.body)
			if w.Code != http.StatusBadRequest {
				t.Errorf("code = %d, want 400; body: %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestAuthLogoutClearsCookie is the small round trip: login, then
// logout, expect the cookie to be cleared. We assert the Set-Cookie
// header is present and has MaxAge < 0 (the standard "delete now"
// value).
func TestAuthLogoutClearsCookie(t *testing.T) {
	r, db := newAuthTestRouter(t)
	seedUser(t, db, "carol@example.com", "hunter2")

	// Login.
	w := doJSON(t, r, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    "carol@example.com",
		"password": "hunter2",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d", w.Code)
	}
	var token string
	for _, c := range w.Result().Cookies() {
		if c.Name == "opc_session" {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatal("no session cookie returned from login")
	}

	// Logout using the cookie.
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: "opc_session", Value: token})
	logoutW := httptest.NewRecorder()
	r.ServeHTTP(logoutW, logoutReq)
	if logoutW.Code != http.StatusNoContent {
		t.Fatalf("logout code = %d, want 204; body: %s", logoutW.Code, logoutW.Body.String())
	}
	var cleared *http.Cookie
	for _, c := range logoutW.Result().Cookies() {
		if c.Name == "opc_session" {
			cleared = c
			break
		}
	}
	if cleared == nil {
		t.Fatal("logout did not set a clearing opc_session cookie")
	}
	if cleared.MaxAge >= 0 {
		t.Errorf("logout cookie MaxAge = %d, want < 0 (expire now)", cleared.MaxAge)
	}
}

// TestAuthRegisterDisabled confirms the default behavior: even with a
// valid body, the endpoint returns 403 because REGISTRATION_ENABLED
// is unset.
func TestAuthRegisterDisabled(t *testing.T) {
	r, _ := newAuthTestRouter(t)
	// Make sure the env var is not set for this test.
	t.Setenv("REGISTRATION_ENABLED", "")

	w := doJSON(t, r, http.MethodPost, "/api/auth/register", map[string]any{
		"email":    "dave@example.com",
		"password": "hunter2",
		"name":     "Dave",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("register code = %d, want 403; body: %s", w.Code, w.Body.String())
	}
}

// TestAuthRegisterEnabled flips the env var and exercises the happy
// path: 201 + a Set-Cookie.
func TestAuthRegisterEnabled(t *testing.T) {
	r, _ := newAuthTestRouter(t)
	t.Setenv("REGISTRATION_ENABLED", "1")

	w := doJSON(t, r, http.MethodPost, "/api/auth/register", map[string]any{
		"email":    "eve@example.com",
		"password": "hunter2",
		"name":     "Eve",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("register code = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	var body struct {
		User struct {
			ID    uint   `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.User.Email != "eve@example.com" {
		t.Errorf("user.email = %q, want eve@example.com", body.User.Email)
	}
	// Cookie must be set so the freshly registered user is
	// immediately logged in.
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "opc_session" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("register did not set opc_session cookie; got: %v", cookies)
	}
}

// TestAuthRegisterDuplicateEmail covers the 409 branch: two
// registrations with the same email must not both succeed.
func TestAuthRegisterDuplicateEmail(t *testing.T) {
	r, _ := newAuthTestRouter(t)
	t.Setenv("REGISTRATION_ENABLED", "true")

	body := map[string]any{
		"email":    "frank@example.com",
		"password": "hunter2",
		"name":     "Frank",
	}
	first := doJSON(t, r, http.MethodPost, "/api/auth/register", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first register code = %d, want 201; body: %s", first.Code, first.Body.String())
	}
	second := doJSON(t, r, http.MethodPost, "/api/auth/register", body)
	if second.Code != http.StatusConflict {
		t.Fatalf("second register code = %d, want 409; body: %s", second.Code, second.Body.String())
	}
}

// TestAuthRegisterEmailCaseInsensitive guards the case-normalization
// path on the registration handler: the handler lowercases the email
// before persistence, so two registrations that differ only in case
// (FRANK@... vs frank@...) must collapse to one row, and the second
// request must 409 instead of producing a duplicate user. This
// complements the strict-equality dedup test by exercising the normal
// way users actually type the same email twice — with different caps.
func TestAuthRegisterEmailCaseInsensitive(t *testing.T) {
	r, _ := newAuthTestRouter(t)
	t.Setenv("REGISTRATION_ENABLED", "true")

	first := doJSON(t, r, http.MethodPost, "/api/auth/register", map[string]any{
		"email":    "Mixed.Case@example.com",
		"password": "hunter2",
		"name":     "Mixed",
	})
	if first.Code != http.StatusCreated {
		t.Fatalf("first register code = %d, want 201; body: %s", first.Code, first.Body.String())
	}
	second := doJSON(t, r, http.MethodPost, "/api/auth/register", map[string]any{
		"email":    "mixed.case@example.com",
		"password": "different",
		"name":     "Lower",
	})
	if second.Code != http.StatusConflict {
		t.Fatalf("case-variant register code = %d, want 409; body: %s", second.Code, second.Body.String())
	}
	// And the same caps-variation must let us log in with the lowercased
	// email — the canonical stored form. This catches a regression where
	// the dedup path is fixed but the lookup path is not.
	loginW := doJSON(t, r, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    "MIXED.case@example.com",
		"password": "hunter2",
	})
	if loginW.Code != http.StatusOK {
		t.Errorf("login (uppercase variant) code = %d, want 200; body: %s", loginW.Code, loginW.Body.String())
	}
}

// TestAuthMeWithValidCookie: log in, then GET /api/auth/me with the
// returned cookie. Expect 200 + the user.
func TestAuthMeWithValidCookie(t *testing.T) {
	r, db := newAuthTestRouter(t)
	u := seedUser(t, db, "gina@example.com", "hunter2")

	w := doJSON(t, r, http.MethodPost, "/api/auth/login", map[string]any{
		"email":    "gina@example.com",
		"password": "hunter2",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d", w.Code)
	}
	var token string
	for _, c := range w.Result().Cookies() {
		if c.Name == "opc_session" {
			token = c.Value
		}
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(&http.Cookie{Name: "opc_session", Value: token})
	meW := httptest.NewRecorder()
	r.ServeHTTP(meW, meReq)
	if meW.Code != http.StatusOK {
		t.Fatalf("me code = %d, want 200; body: %s", meW.Code, meW.Body.String())
	}
	var body struct {
		User struct {
			ID    uint   `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(meW.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.User.ID != u.ID {
		t.Errorf("me id = %d, want %d", body.User.ID, u.ID)
	}
}

// TestAuthMeWithoutCookie: no cookie = 401.
func TestAuthMeWithoutCookie(t *testing.T) {
	r, _ := newAuthTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("me (no cookie) code = %d, want 401; body: %s", w.Code, w.Body.String())
	}
}

// TestRequireAuthRejectsExpiredCookie covers the missing gap in the
// middleware coverage matrix: a request that carries a well-formed
// session token whose ExpiresAt is in the past must NOT pass through
// to the protected handler, and the response body must be the same
// canonical 401 envelope RequireAuth emits for the no-cookie case
// (so a client cannot distinguish "expired" from "unauthenticated"
// and probe for stale-token oracle behaviour).
//
// We seed the row directly (rather than going through
// auth.CreateSession and waiting 7 days) so the test is fast and
// deterministic, and so it doesn't depend on time-mocking helpers.
func TestRequireAuthRejectsExpiredCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := newAuthTestDB(t)
	u := seedUser(t, gormDB, "ivan@example.com", "hunter2")

	// Manually create a session row that is already expired. The
	// middleware reads ValidateSession which checks ExpiresAt, so
	// this is the closest you can get to "the user logged in last
	// week and the cookie is still in their jar" without sleeping
	// for a week.
	token := "expired-token-" + u.Email
	expired := models.Session{
		UserID:    u.ID,
		Token:     token,
		ExpiresAt: time.Now().UTC().Add(-1 * time.Hour),
	}
	if err := gormDB.Create(&expired).Error; err != nil {
		t.Fatalf("seed expired session: %v", err)
	}

	var downstreamHit bool
	r := gin.New()
	r.GET("/api/protected",
		NewRequireAuth(gormDB, nil),
		func(c *gin.Context) {
			downstreamHit = true
			c.JSON(http.StatusOK, gin.H{"ok": true})
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: "opc_session", Value: token})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("require auth (expired cookie) code = %d, want 401; body: %s", w.Code, w.Body.String())
	}
	if downstreamHit {
		t.Errorf("downstream handler ran despite expired session; RequireAuth must short-circuit")
	}
	if !strings.Contains(w.Body.String(), "not authenticated") {
		t.Errorf("body = %q, want the canonical 'not authenticated' envelope", w.Body.String())
	}

	// And the row should have been best-effort deleted by
	// ValidateSession on the expired path, so a second request with
	// the same token behaves like an unknown token (404 in the
	// session lookup) — still 401, but with the not-found branch
	// in the log.
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("require auth (replayed expired cookie) code = %d, want 401; body: %s", w2.Code, w2.Body.String())
	}
}

// TestRequireAuthBlocks verifies the middleware: a request through a
// route protected by RequireAuth, with no cookie, must 401.
func TestRequireAuthBlocks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := newAuthTestDB(t)
	r := gin.New()
	r.GET("/api/protected",
		NewRequireAuth(gormDB, nil),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("require auth (no cookie) code = %d, want 401; body: %s", w.Code, w.Body.String())
	}
}

// TestRequireAuthAllows: with a valid session cookie, the middleware
// must call c.Next() so the protected handler runs and can read the
// user off the context.
func TestRequireAuthAllows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := newAuthTestDB(t)
	u := seedUser(t, gormDB, "hugo@example.com", "hunter2")
	s, err := auth.CreateSession(gormDB, u.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	r := gin.New()
	r.GET("/api/protected",
		NewRequireAuth(gormDB, nil),
		func(c *gin.Context) {
			id := UserIDFromContext(c)
			c.JSON(http.StatusOK, gin.H{"user_id": id})
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: "opc_session", Value: s.Token})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("require auth (valid cookie) code = %d, want 200; body: %s", w.Code, w.Body.String())
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
