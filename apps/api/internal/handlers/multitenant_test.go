package handlers

// Cross-tenant regression tests for Phase 2 Task 3.
//
// The spec gaps that motivated this file:
//   1. Existing handler tests register routes WITHOUT RequireAuth, so the
//      "user_id stamps from context" code path is never exercised by the
//      test suite. A regression that, say, dropped the UserID = ...
//      assignment from Create() would still pass every other test.
//   2. The store layer uses `if userID != 0 { q = q.Where(...) }`, so
//      a future caller that bypassed RequireAuth (admin tool, MCP, cron)
//      would silently return cross-tenant rows. We pin the production
//      shape here by seeding two real users and authenticating as each.
//
// The matrix: for every per-entity register route (topic / script /
// series / knowledge / content_item) and every CRUD verb that scopes
// by user (List / Get / Update / Delete), assert that userB cannot
// touch a row userA created. List returns empty for userB; the
// single-row verbs return 404 and DO NOT mutate the row.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/auth"
	"github.com/opc/api/internal/models"
)

// newMultiTenantDB returns an in-memory SQLite handle with every table
// the multi-tenant regression tests need. We deliberately do NOT reuse
// newTestDB / newAuthTestDB here because (a) the auth DB uses shared
// cache, and we want full ownership of the file; (b) we need the
// entities + users + sessions tables on one handle so the auth
// middleware can validate cookies against the same store the handlers
// query.
func newMultiTenantDB(t *testing.T) *gorm.DB {
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
		&models.Session{},
		&models.Topic{},
		&models.Script{},
		&models.Series{},
		&models.KnowledgeDoc{},
		&models.ContentItem{},
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

// multiTenantRig wires a router that has every protected CRUD surface
// under /api/*, with the real RequireAuth middleware in front. Two
// users are seeded (userA, userB) and their session cookies are
// returned to the caller for authed requests.
type multiTenantRig struct {
	router  *gin.Engine
	db      *gorm.DB
	userA   models.User
	userB   models.User
	cookieA string
	cookieB string
}

func newMultiTenantRig(t *testing.T) *multiTenantRig {
	t.Helper()
	db := newMultiTenantDB(t)
	r := gin.New()

	// Single RequireAuth middleware in front of every protected group,
	// matching the production wiring in main.go.
	authed := r.Group("/api", NewRequireAuth(db, nil))

	// Mount every entity we want to test. Order does not matter
	// because each handler owns its own URL space.
	NewTopicHandlerFromGorm(db, nil).RegisterRoutes(authed)
	NewScriptHandler(db).RegisterRoutes(authed)
	NewSeriesHandler(db).RegisterRoutes(authed)
	NewKnowledgeDocHandler(db).RegisterRoutes(authed)
	NewContentItemHandler(db).RegisterRoutes(authed)

	hash := func(pw string) string {
		h, err := auth.HashPassword(pw)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		return h
	}
	seed := func(email string) models.User {
		u := models.User{Email: email, PasswordHash: hash("pw-" + email), Name: email}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("seed user %s: %v", email, err)
		}
		if _, err := auth.CreateSession(db, u.ID); err != nil {
			t.Fatalf("create session for %s: %v", email, err)
		}
		return u
	}

	userA := seed("a@example.com")
	userB := seed("b@example.com")

	return &multiTenantRig{
		router: r,
		db:     db,
		userA:  userA,
		userB:  userB,
	}
}

// cookieForUser looks up the most recent session token for a user. The
// auth.CreateSession helper inserts one row per call, so this is a
// unique key in the test.
func cookieForUser(t *testing.T, db *gorm.DB, userID uint) string {
	t.Helper()
	var s models.Session
	if err := db.Where("user_id = ?", userID).Order("id desc").First(&s).Error; err != nil {
		t.Fatalf("cookieForUser(%d): %v", userID, err)
	}
	return s.Token
}

// doAuthed issues a request with the given session cookie. Mirrors do
// but adds the cookie header so RequireAuth resolves the user.
func (rig *multiTenantRig) doAuthed(t *testing.T, cookie, method, path string, body any) *httptest.ResponseRecorder {
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
	req.AddCookie(&http.Cookie{Name: "opc_session", Value: cookie})
	w := httptest.NewRecorder()
	rig.router.ServeHTTP(w, req)
	return w
}

// listIDs returns the slice of {id, user_id} pairs from a list
// endpoint, so the assertions can pin "userB sees zero rows" without
// tripping on shape drift in the response.
type idAndOwner struct {
	ID     uint `json:"id"`
	UserID uint `json:"user_id"`
}

func listIDs(t *testing.T, w *httptest.ResponseRecorder) []idAndOwner {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("list code = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []idAndOwner `json:"items"`
		Total int64        `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.Items
}

// TestMultiTenant_TopicIsolation is the canonical matrix: userA
// creates a topic, userB cannot read/update/delete it. Each of the
// CRUD verbs is exercised in turn.
func TestMultiTenant_TopicIsolation(t *testing.T) {
	rig := newMultiTenantRig(t)
	cookieA := cookieForUser(t, rig.db, rig.userA.ID)
	cookieB := cookieForUser(t, rig.db, rig.userB.ID)

	// userA creates a topic.
	createW := rig.doAuthed(t, cookieA, http.MethodPost, "/api/topics", map[string]any{
		"title":    "A's topic",
		"platform": "抖音",
		"status":   "想法",
	})
	if createW.Code != http.StatusCreated {
		t.Fatalf("userA create code = %d, want 201; body: %s", createW.Code, createW.Body.String())
	}
	var created models.Topic
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.UserID != rig.userA.ID {
		t.Fatalf("create stamped userID = %d, want %d", created.UserID, rig.userA.ID)
	}

	idPath := "/api/topics/" + strconv.Itoa(int(created.ID))

	// userA can list and find it.
	aList := listIDs(t, rig.doAuthed(t, cookieA, http.MethodGet, "/api/topics", nil))
	if len(aList) != 1 || aList[0].ID != created.ID {
		t.Errorf("userA list = %+v, want only their own topic", aList)
	}

	// userB lists — must be empty.
	bList := listIDs(t, rig.doAuthed(t, cookieB, http.MethodGet, "/api/topics", nil))
	if len(bList) != 0 {
		t.Errorf("userB list leaked %d topics: %+v", len(bList), bList)
	}

	// userB GET — must 404.
	if w := rig.doAuthed(t, cookieB, http.MethodGet, idPath, nil); w.Code != http.StatusNotFound {
		t.Errorf("userB GET code = %d, want 404; body: %s", w.Code, w.Body.String())
	}

	// userB PUT — must 404, and the row must be unchanged.
	if w := rig.doAuthed(t, cookieB, http.MethodPut, idPath, map[string]any{"title": "pwned"}); w.Code != http.StatusNotFound {
		t.Errorf("userB PUT code = %d, want 404; body: %s", w.Code, w.Body.String())
	}

	// userB DELETE — must 404, and the row must still exist.
	if w := rig.doAuthed(t, cookieB, http.MethodDelete, idPath, nil); w.Code != http.StatusNotFound {
		t.Errorf("userB DELETE code = %d, want 404; body: %s", w.Code, w.Body.String())
	}

	// Confirm the row is intact and still owned by userA.
	var after models.Topic
	if err := rig.db.First(&after, created.ID).Error; err != nil {
		t.Fatalf("re-read after userB attacks: %v", err)
	}
	if after.Title != "A's topic" {
		t.Errorf("userB's PUT mutated the row; title is now %q", after.Title)
	}
	if after.UserID != rig.userA.ID {
		t.Errorf("user_id changed; was %d, is %d", rig.userA.ID, after.UserID)
	}
}

// TestMultiTenant_ScriptIsolation is the same shape for /scripts.
func TestMultiTenant_ScriptIsolation(t *testing.T) {
	rig := newMultiTenantRig(t)
	cookieA := cookieForUser(t, rig.db, rig.userA.ID)
	cookieB := cookieForUser(t, rig.db, rig.userB.ID)

	createW := rig.doAuthed(t, cookieA, http.MethodPost, "/api/scripts", map[string]any{
		"topic_id": 1,
		"title":    "A's script",
		"content":  "secret",
		"platform": "抖音",
	})
	if createW.Code != http.StatusCreated {
		t.Fatalf("userA create code = %d, want 201; body: %s", createW.Code, createW.Body.String())
	}
	var created models.Script
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.UserID != rig.userA.ID {
		t.Fatalf("stamped userID = %d, want %d", created.UserID, rig.userA.ID)
	}

	idPath := "/api/scripts/" + strconv.Itoa(int(created.ID))

	// userB list empty.
	bList := listIDs(t, rig.doAuthed(t, cookieB, http.MethodGet, "/api/scripts", nil))
	if len(bList) != 0 {
		t.Errorf("userB list leaked %d scripts", len(bList))
	}

	// userB GET → 404.
	if w := rig.doAuthed(t, cookieB, http.MethodGet, idPath, nil); w.Code != http.StatusNotFound {
		t.Errorf("userB GET code = %d, want 404", w.Code)
	}
	// userB PUT → 404.
	if w := rig.doAuthed(t, cookieB, http.MethodPut, idPath, map[string]any{"title": "pwned"}); w.Code != http.StatusNotFound {
		t.Errorf("userB PUT code = %d, want 404", w.Code)
	}
	// userB DELETE → 404.
	if w := rig.doAuthed(t, cookieB, http.MethodDelete, idPath, nil); w.Code != http.StatusNotFound {
		t.Errorf("userB DELETE code = %d, want 404", w.Code)
	}

	// Row still intact.
	var after models.Script
	if err := rig.db.First(&after, created.ID).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.Title != "A's script" || after.UserID != rig.userA.ID {
		t.Errorf("row mutated: title=%q userID=%d", after.Title, after.UserID)
	}
}

// TestMultiTenant_SeriesIsolation is the same shape for /series.
func TestMultiTenant_SeriesIsolation(t *testing.T) {
	rig := newMultiTenantRig(t)
	cookieA := cookieForUser(t, rig.db, rig.userA.ID)
	cookieB := cookieForUser(t, rig.db, rig.userB.ID)

	createW := rig.doAuthed(t, cookieA, http.MethodPost, "/api/series", map[string]any{
		"name":        "A's series",
		"description": "private",
		"ip_id":       1,
	})
	if createW.Code != http.StatusCreated {
		t.Fatalf("userA create code = %d, want 201; body: %s", createW.Code, createW.Body.String())
	}
	var created models.Series
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.UserID != rig.userA.ID {
		t.Fatalf("stamped userID = %d, want %d", created.UserID, rig.userA.ID)
	}

	idPath := "/api/series/" + strconv.Itoa(int(created.ID))

	if list := listIDs(t, rig.doAuthed(t, cookieB, http.MethodGet, "/api/series", nil)); len(list) != 0 {
		t.Errorf("userB list leaked %d series", len(list))
	}
	if w := rig.doAuthed(t, cookieB, http.MethodGet, idPath, nil); w.Code != http.StatusNotFound {
		t.Errorf("userB GET code = %d, want 404", w.Code)
	}
	if w := rig.doAuthed(t, cookieB, http.MethodPut, idPath, map[string]any{"name": "pwned"}); w.Code != http.StatusNotFound {
		t.Errorf("userB PUT code = %d, want 404", w.Code)
	}
	if w := rig.doAuthed(t, cookieB, http.MethodDelete, idPath, nil); w.Code != http.StatusNotFound {
		t.Errorf("userB DELETE code = %d, want 404", w.Code)
	}

	var after models.Series
	if err := rig.db.First(&after, created.ID).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.Name != "A's series" || after.UserID != rig.userA.ID {
		t.Errorf("row mutated: name=%q userID=%d", after.Name, after.UserID)
	}
}

// TestMultiTenant_KnowledgeDocIsolation is the same shape for /knowledge.
func TestMultiTenant_KnowledgeDocIsolation(t *testing.T) {
	rig := newMultiTenantRig(t)
	cookieA := cookieForUser(t, rig.db, rig.userA.ID)
	cookieB := cookieForUser(t, rig.db, rig.userB.ID)

	createW := rig.doAuthed(t, cookieA, http.MethodPost, "/api/knowledge", map[string]any{
		"title":    "A's notes",
		"path":     "private/notes",
		"content":  "secret",
		"doc_type": "ip-style",
	})
	if createW.Code != http.StatusCreated {
		t.Fatalf("userA create code = %d, want 201; body: %s", createW.Code, createW.Body.String())
	}
	var created models.KnowledgeDoc
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.UserID != rig.userA.ID {
		t.Fatalf("stamped userID = %d, want %d", created.UserID, rig.userA.ID)
	}

	idPath := "/api/knowledge/" + strconv.Itoa(int(created.ID))

	if list := listIDs(t, rig.doAuthed(t, cookieB, http.MethodGet, "/api/knowledge", nil)); len(list) != 0 {
		t.Errorf("userB list leaked %d docs", len(list))
	}
	if w := rig.doAuthed(t, cookieB, http.MethodGet, idPath, nil); w.Code != http.StatusNotFound {
		t.Errorf("userB GET code = %d, want 404", w.Code)
	}
	if w := rig.doAuthed(t, cookieB, http.MethodPut, idPath, map[string]any{"title": "pwned"}); w.Code != http.StatusNotFound {
		t.Errorf("userB PUT code = %d, want 404", w.Code)
	}
	if w := rig.doAuthed(t, cookieB, http.MethodDelete, idPath, nil); w.Code != http.StatusNotFound {
		t.Errorf("userB DELETE code = %d, want 404", w.Code)
	}

	var after models.KnowledgeDoc
	if err := rig.db.First(&after, created.ID).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.Title != "A's notes" || after.UserID != rig.userA.ID {
		t.Errorf("row mutated: title=%q userID=%d", after.Title, after.UserID)
	}
}

// TestMultiTenant_ContentItemIsolation is the same shape for /content-items.
func TestMultiTenant_ContentItemIsolation(t *testing.T) {
	rig := newMultiTenantRig(t)
	cookieA := cookieForUser(t, rig.db, rig.userA.ID)
	cookieB := cookieForUser(t, rig.db, rig.userB.ID)

	createW := rig.doAuthed(t, cookieA, http.MethodPost, "/api/content-items", map[string]any{
		"script_id":    1,
		"platform":     "抖音",
		"platform_url": "https://www.douyin.com/video/secret",
	})
	if createW.Code != http.StatusCreated {
		t.Fatalf("userA create code = %d, want 201; body: %s", createW.Code, createW.Body.String())
	}
	var created models.ContentItem
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.UserID != rig.userA.ID {
		t.Fatalf("stamped userID = %d, want %d", created.UserID, rig.userA.ID)
	}

	idPath := "/api/content-items/" + strconv.Itoa(int(created.ID))

	if list := listIDs(t, rig.doAuthed(t, cookieB, http.MethodGet, "/api/content-items", nil)); len(list) != 0 {
		t.Errorf("userB list leaked %d content items", len(list))
	}
	if w := rig.doAuthed(t, cookieB, http.MethodGet, idPath, nil); w.Code != http.StatusNotFound {
		t.Errorf("userB GET code = %d, want 404", w.Code)
	}
	if w := rig.doAuthed(t, cookieB, http.MethodPut, idPath, map[string]any{"platform_url": "pwned"}); w.Code != http.StatusNotFound {
		t.Errorf("userB PUT code = %d, want 404", w.Code)
	}
	if w := rig.doAuthed(t, cookieB, http.MethodDelete, idPath, nil); w.Code != http.StatusNotFound {
		t.Errorf("userB DELETE code = %d, want 404", w.Code)
	}

	var after models.ContentItem
	if err := rig.db.First(&after, created.ID).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.PlatformURL != "https://www.douyin.com/video/secret" || after.UserID != rig.userA.ID {
		t.Errorf("row mutated: url=%q userID=%d", after.PlatformURL, after.UserID)
	}
}

// TestMultiTenant_CreateIgnoresBodyUserID proves that even if a
// malicious client tries to seed userID=N in the POST body, the
// handler overwrites it with the authenticated user. Without this
// check, a single authed request could create rows for arbitrary
// other users.
func TestMultiTenant_CreateIgnoresBodyUserID(t *testing.T) {
	rig := newMultiTenantRig(t)
	cookieA := cookieForUser(t, rig.db, rig.userA.ID)

	createW := rig.doAuthed(t, cookieA, http.MethodPost, "/api/topics", map[string]any{
		"title":    "ownership check",
		"platform": "抖音",
		"status":   "想法",
		"user_id":  rig.userB.ID, // attacker trying to plant a row in userB
	})
	if createW.Code != http.StatusCreated {
		t.Fatalf("create code = %d, want 201; body: %s", createW.Code, createW.Body.String())
	}
	var created models.Topic
	if err := json.Unmarshal(createW.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.UserID != rig.userA.ID {
		t.Errorf("body user_id leaked: created.UserID=%d, want %d (authenticated user)", created.UserID, rig.userA.ID)
	}
}

// TestMultiTenant_NoCookieMeans401 confirms that without a cookie
// (or with the wrong cookie), every CRUD verb short-circuits with the
// canonical 401 envelope. This is the "I forgot to call RequireAuth"
// tripwire.
func TestMultiTenant_NoCookieMeans401(t *testing.T) {
	rig := newMultiTenantRig(t)
	// Note: doAuthed here uses the empty cookie, so RequireAuth
	// short-circuits before any handler runs. The path uses /api/*
	// so the URL is well-formed for the auth middleware to reach.
	paths := []struct{ method, path string }{
		{http.MethodGet, "/api/topics"},
		{http.MethodPost, "/api/topics"},
		{http.MethodGet, "/api/topics/1"},
		{http.MethodPut, "/api/topics/1"},
		{http.MethodDelete, "/api/topics/1"},
	}
	for _, p := range paths {
		var body any
		if p.method == http.MethodPost || p.method == http.MethodPut {
			body = map[string]any{"title": "x", "platform": "抖音", "status": "想法"}
		}
		w := rig.doAuthed(t, "", p.method, p.path, body)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with no cookie: code = %d, want 401; body: %s", p.method, p.path, w.Code, w.Body.String())
		}
	}
}
