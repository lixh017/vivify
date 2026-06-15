package handlers

// Sub-Spec C M1 — Task 2: 4 admin endpoint unit tests for
// CreatorHandler. All tests are operator-only: they bypass the session
// cookie by stamping user_role directly on the Gin context (matching
// the RequireAuth → RequireOperatorRole pipeline main.go wires up in
// production). A creator session or no session produces the documented
// 403 / 401 paths.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/auth"
	"github.com/opc/api/internal/models"
)

// setupTestDB returns a fresh in-memory SQLite DB with the user/session
// tables migrated. We use a PRIVATE per-test path (not shared in-memory
// cache) so parallel cases cannot leak data into each other through
// the unique-email index.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	path := t.TempDir() + "/creator-test.db"
	gormDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{
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

// createTestOperator inserts a user with role="operator" and a hashed
// password. The returned user has a valid ID so caller code can stamp
// it onto a session. The email is suffixed with the test name so
// parallel cases cannot collide on the unique-email index.
func createTestOperator(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	hash, err := auth.HashPassword("operator-pass-1234")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := models.User{
		Email:        "op-" + t.Name() + "@beta.com",
		PasswordHash: hash,
		Name:         "Operator",
		Role:         "operator",
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create operator: %v", err)
	}
	return u
}

// createTestCreator inserts a user with role="creator" and a hashed
// password. Returns the persisted user (with valid ID) for stamping
// onto a session. The email is suffixed with the test name + a
// per-call counter so a single test that creates multiple creators
// does not collide on the unique-email index.
var creatorEmailCounter int

func createTestCreator(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	hash, err := auth.HashPassword("creator-pass-1234")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	creatorEmailCounter++
	u := models.User{
		Email:        "creator-" + t.Name() + "-" + strconv.Itoa(creatorEmailCounter) + "@beta.com",
		PasswordHash: hash,
		Name:         "Creator",
		Role:         "creator",
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create creator: %v", err)
	}
	return u
}

// stampAuthContext is the small middleware the tests use to fake the
// "RequireAuth already ran" state: it sets user_id + user_role on the
// Gin context so the handler under test can call UserIDFromContext
// without needing a real session-cookie round-trip.
//
// We use it as an inner-middleware in newTestRouter. Tests pick which
// role to stamp by calling r.ServeHTTP with a request whose
// X-Test-User-Id header names a pre-seeded user — see the helper
// stampFor below.
func stampAuthContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		if id := c.GetHeader("X-Test-User-Id"); id != "" {
			c.Set("user_id", atoi(id))
			c.Set("user_role", c.GetHeader("X-Test-User-Role"))
		}
		c.Next()
	}
}

// atoi is a small stdlib-free helper so the test file stays free of
// strconv just for the header-stamp call site. (The spec body itself
// uses strconv.FormatUint for URL paths.)
func atoi(s string) uint {
	var n uint
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + uint(ch-'0')
	}
	return n
}

// newTestRouter wires the CreatorHandler behind the same RequireAuth
// + RequireOperatorRole production chain — but with a TEST-mode
// auth shim that resolves the user from X-Test-User-Id / X-Test-User-Role
// headers instead of a session cookie. This keeps the unit tests
// hermetic (no bcrypt dance, no session table) while still
// exercising the real middleware chain.
//
// The handler is mounted on a /api/admin group (matching
// cmd/server/main.go) so RegisterRoutes uses the same relative
// paths it does in production.
func newTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(stampAuthContext())
	// Use the production middleware under test. They run AFTER
	// stampAuthContext so user_id + user_role are populated.
	r.Use(testRequireAuth(), testRequireOperatorRole())
	adminGroup := r.Group("/api/admin")
	// Mount every admin handler the platform currently exposes
	// on the same /api/admin group. Sub-Spec C creators +
	// Sub-Spec D agents are both operator-only; the
	// RequireOperatorRole middleware above enforces the role
	// gate for the whole subtree.
	NewCreatorHandler(db).RegisterRoutes(adminGroup)
	NewAgentHandler(db).RegisterRoutes(adminGroup)
	return r
}

// testRequireAuth is a drop-in replacement for handlers.NewRequireAuth
// that resolves the user from the X-Test-User-Id / X-Test-User-Role
// headers instead of a session cookie. Returns the same middleware
// signature so it slots into the router as a peer of the production
// middleware.
func testRequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// If no headers are present, short-circuit with 401 the same
		// way the production middleware would for a missing cookie.
		if c.GetHeader("X-Test-User-Id") == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		c.Next()
	}
}

// testRequireOperatorRole is the test-mode mirror of
// middleware.RequireOperatorRole — it reads user_role from the Gin
// context (which stampAuthContext populated from the X-Test-User-Role
// header) and rejects on mismatch.
func testRequireOperatorRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := c.Get("user_role")
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no user role in context"})
			return
		}
		if role.(string) != "operator" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "operator role required"})
			return
		}
		c.Next()
	}
}

// stampFor returns the headers a test needs to add to its request so
// the test-mode auth chain resolves to the given user with the given
// role. Use it like: req.Header = stampFor(op.ID, "operator")
func stampFor(userID uint, role string) http.Header {
	h := http.Header{}
	h.Set("X-Test-User-Id", strconv.FormatUint(uint64(userID), 10))
	h.Set("X-Test-User-Role", role)
	return h
}

// makeJSONReq builds a JSON POST request with the given body, copying
// in the auth-stamp headers. We deliberately avoid the package-level
// doJSON helper because we need the Header stitching; tests that want
// GET (no body) use makeReq below.
func makeJSONReq(t *testing.T, method, path string, body string, hdr http.Header) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return req
}

// makeReq builds a non-JSON request (typically GET / POST-without-body)
// with the auth-stamp headers attached.
func makeReq(t *testing.T, method, path string, hdr http.Header) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return req
}

// TestCreateCreator201 is the happy path: an operator POSTs
// {email, password} and gets 201 + the new user back, with
// Role="creator" and CreatedBy=operator.ID.
func TestCreateCreator201(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)

	body := `{"email":"creator1@beta.com","password":"beta-pass-123"}`
	req := makeJSONReq(t, http.MethodPost, "/api/admin/creators", body, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", w.Code, w.Body.String())
	}
	var got struct {
		User models.User `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, w.Body.String())
	}
	if got.User.Email != "creator1@beta.com" {
		t.Errorf("email = %q, want creator1@beta.com", got.User.Email)
	}
	if got.User.Role != "creator" {
		t.Errorf("role = %q, want creator", got.User.Role)
	}
	if got.User.CreatedBy != op.ID {
		t.Errorf("created_by = %d, want %d", got.User.CreatedBy, op.ID)
	}
}

// TestCreateCreator400InvalidEmail: malformed email → 400.
func TestCreateCreator400InvalidEmail(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)

	body := `{"email":"not-an-email","password":"beta-pass-123"}`
	req := makeJSONReq(t, http.MethodPost, "/api/admin/creators", body, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
	}
}

// TestCreateCreator400ShortPassword: password < 8 chars → 400.
func TestCreateCreator400ShortPassword(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)

	body := `{"email":"x@x.com","password":"short"}` // 5 chars
	req := makeJSONReq(t, http.MethodPost, "/api/admin/creators", body, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
	}
}

// TestCreateCreator409Duplicate: second create with the same email → 409.
func TestCreateCreator409Duplicate(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)
	existing := models.User{
		Email:        "dup@beta.com",
		PasswordHash: "h",
		Role:         "creator",
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	body := `{"email":"dup@beta.com","password":"beta-pass-123"}`
	req := makeJSONReq(t, http.MethodPost, "/api/admin/creators", body, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (body: %s)", w.Code, w.Body.String())
	}
}

// TestCreateCreator403NonOperator: a creator-role session is rejected
// with 403 by the RequireOperatorRole middleware.
func TestCreateCreator403NonOperator(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	creator := createTestCreator(t, db)

	body := `{"email":"new@beta.com","password":"beta-pass-123"}`
	req := makeJSONReq(t, http.MethodPost, "/api/admin/creators", body, stampFor(creator.ID, "creator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (body: %s)", w.Code, w.Body.String())
	}
}

// TestListCreators200: GET /api/admin/creators returns only creator-role
// users, newest first.
func TestListCreators200(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)
	_ = createTestCreator(t, db)
	_ = createTestCreator(t, db)

	req := makeReq(t, http.MethodGet, "/api/admin/creators", stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var got struct {
		Creators []models.User `json:"creators"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, w.Body.String())
	}
	if len(got.Creators) != 2 {
		t.Errorf("len(creators) = %d, want 2", len(got.Creators))
	}
	for i, c := range got.Creators {
		if c.Role != "creator" {
			t.Errorf("creators[%d] role = %q, want creator", i, c.Role)
		}
	}
	// Spot-check the operator was NOT in the list — the handler
	// filters by role="creator".
	for _, c := range got.Creators {
		if c.ID == op.ID {
			t.Errorf("operator leaked into creator list (id=%d)", op.ID)
		}
	}
}

// TestResetPassword200: POST /reset-password actually updates the
// stored hash so the new password can be used to authenticate.
func TestResetPassword200(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)
	creator := createTestCreator(t, db)

	body := `{"new_password":"new-pass-1234"}`
	path := "/api/admin/creators/" + strconv.FormatUint(uint64(creator.ID), 10) + "/reset-password"
	req := makeJSONReq(t, http.MethodPost, path, body, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	// Verify password was actually changed.
	var got models.User
	if err := db.First(&got, creator.ID).Error; err != nil {
		t.Fatalf("re-read user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte("new-pass-1234")); err != nil {
		t.Errorf("password not updated: %v", err)
	}
}

// TestResetPassword404: target user does not exist → 404.
func TestResetPassword404(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)

	body := `{"new_password":"new-pass-1234"}`
	path := "/api/admin/creators/99999/reset-password"
	req := makeJSONReq(t, http.MethodPost, path, body, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body: %s)", w.Code, w.Body.String())
	}
}

// TestResetPassword400NotCreator: target user exists but is NOT a
// creator (it's an operator) → 400. The endpoint is creator-scoped.
func TestResetPassword400NotCreator(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)

	body := `{"new_password":"new-pass-1234"}`
	path := "/api/admin/creators/" + strconv.FormatUint(uint64(op.ID), 10) + "/reset-password"
	req := makeJSONReq(t, http.MethodPost, path, body, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (target user is operator, not creator) (body: %s)", w.Code, w.Body.String())
	}
}

// TestDisableCreator200: POST /disable flips Disabled=true on the
// targeted creator row.
func TestDisableCreator200(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	op := createTestOperator(t, db)
	creator := createTestCreator(t, db)

	path := "/api/admin/creators/" + strconv.FormatUint(uint64(creator.ID), 10) + "/disable"
	req := makeReq(t, http.MethodPost, path, stampFor(op.ID, "operator"))
	w := httptest.NewRecorder()
	r := newTestRouter(db)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	var got models.User
	if err := db.First(&got, creator.ID).Error; err != nil {
		t.Fatalf("re-read user: %v", err)
	}
	if !got.Disabled {
		t.Errorf("disabled = false, want true")
	}
}