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

	"github.com/opc/api/internal/models"
)

// newTestDB returns an in-memory SQLite GORM handle and registers a t.Cleanup
// hook to close the underlying *sql.DB once the test finishes. This avoids
// the leak that occurred when each test re-migrated a fresh DB.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := gormDB.AutoMigrate(&models.Topic{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return gormDB
}

func setupTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	gormDB := newTestDB(t)
	r := gin.New()
	h := NewTopicHandlerFromGorm(gormDB, nil)
	h.RegisterRoutes(r)
	return r, gormDB
}

// do issues an in-memory HTTP request against r. Body is JSON-encoded when
// non-nil. The returned recorder is the response.
func do(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

func TestTopicCRUD(t *testing.T) {
	type seed struct {
		title    string
		platform string
		status   string
	}
	type want struct {
		code int
		// body assertions: each fn inspects the parsed response
		check func(t *testing.T, body []byte, code int, all []models.Topic)
	}
	cases := []struct {
		name  string
		seeds []seed
		steps func(t *testing.T, r *gin.Engine, ids []uint) []struct {
			step         string
			method, path string
			body         any
			want         want
		}
	}{
		{
			name: "happy path: create, list, get, update, delete",
			steps: func(t *testing.T, r *gin.Engine, _ []uint) []struct {
				step         string
				method, path string
				body         any
				want         want
			} {
				return []struct {
					step         string
					method, path string
					body         any
					want         want
				}{
					{
						step: "create", method: "POST", path: "/topics",
						body: map[string]any{
							"title": "测试选题", "angle": "反常识",
							"platform": "抖音", "status": "想法",
						},
						want: want{
							code: http.StatusCreated,
							check: func(t *testing.T, b []byte, _ int, _ []models.Topic) {
								var tp models.Topic
								if err := json.Unmarshal(b, &tp); err != nil {
									t.Fatalf("decode: %v", err)
								}
								if tp.Title != "测试选题" {
									t.Errorf("title: %q", tp.Title)
								}
								if tp.ID == 0 {
									t.Errorf("expected non-zero ID")
								}
							},
						},
					},
					{
						step: "list shows 1", method: "GET", path: "/topics",
						want: want{
							code: http.StatusOK,
							check: func(t *testing.T, b []byte, _ int, _ []models.Topic) {
								var resp struct {
									Items []models.Topic `json:"items"`
									Total int64          `json:"total"`
								}
								if err := json.Unmarshal(b, &resp); err != nil {
									t.Fatalf("decode: %v", err)
								}
								if resp.Total != 1 || len(resp.Items) != 1 {
									t.Errorf("expected 1 topic, got total=%d items=%d", resp.Total, len(resp.Items))
								}
							},
						},
					},
					{
						step: "get by id", method: "GET", path: "/topics/1",
						want: want{
							code: http.StatusOK,
							check: func(t *testing.T, b []byte, _ int, _ []models.Topic) {
								var tp models.Topic
								if err := json.Unmarshal(b, &tp); err != nil {
									t.Fatalf("decode: %v", err)
								}
								if tp.ID != 1 {
									t.Errorf("id: %d", tp.ID)
								}
							},
						},
					},
					{
						step: "patch preserves omitted fields", method: "PATCH", path: "/topics/1",
						body: map[string]any{"title": "Updated", "status": "评估"},
						want: want{
							code: http.StatusOK,
							check: func(t *testing.T, b []byte, _ int, _ []models.Topic) {
								var tp models.Topic
								if err := json.Unmarshal(b, &tp); err != nil {
									t.Fatalf("decode: %v", err)
								}
								if tp.Title != "Updated" {
									t.Errorf("title: %q", tp.Title)
								}
								if tp.Status != "评估" {
									t.Errorf("status: %q", tp.Status)
								}
								// angle and platform should be preserved from create
								if tp.Angle != "反常识" {
									t.Errorf("angle not preserved: %q", tp.Angle)
								}
								if tp.Platform != "抖音" {
									t.Errorf("platform not preserved: %q", tp.Platform)
								}
							},
						},
					},
					{
						step: "delete returns 204", method: "DELETE", path: "/topics/1",
						want: want{
							code: http.StatusNoContent,
							check: func(t *testing.T, b []byte, _ int, _ []models.Topic) {
								if len(b) != 0 {
									t.Errorf("expected empty body, got %q", string(b))
								}
							},
						},
					},
					{
						step: "list now empty", method: "GET", path: "/topics",
						want: want{
							code: http.StatusOK,
							check: func(t *testing.T, b []byte, _ int, all []models.Topic) {
								if len(all) != 0 {
									t.Errorf("expected 0 topics, got %d", len(all))
								}
							},
						},
					},
				}
			},
		},
		{
			name: "filter by platform and status",
			seeds: []seed{
				{title: "A", platform: "抖音", status: "想法"},
				{title: "B", platform: "哔哩哔哩", status: "评估"},
				{title: "C", platform: "抖音", status: "已发布"},
			},
			steps: func(t *testing.T, r *gin.Engine, _ []uint) []struct {
				step         string
				method, path string
				body         any
				want         want
			} {
				return []struct {
					step         string
					method, path string
					body         any
					want         want
				}{
					{
						step: "platform=抖音", method: "GET", path: "/topics?platform=抖音",
						want: want{
							code: http.StatusOK,
							check: func(t *testing.T, b []byte, _ int, _ []models.Topic) {
								var resp struct {
									Items []models.Topic `json:"items"`
								}
								if err := json.Unmarshal(b, &resp); err != nil {
									t.Fatalf("decode: %v", err)
								}
								if len(resp.Items) != 2 {
									t.Errorf("expected 2, got %d", len(resp.Items))
								}
								for _, tp := range resp.Items {
									if tp.Platform != "抖音" {
										t.Errorf("platform leak: %q", tp.Platform)
									}
								}
							},
						},
					},
					{
						step: "status=评估", method: "GET", path: "/topics?status=评估",
						want: want{
							code: http.StatusOK,
							check: func(t *testing.T, b []byte, _ int, _ []models.Topic) {
								var resp struct {
									Items []models.Topic `json:"items"`
								}
								_ = json.Unmarshal(b, &resp)
								if len(resp.Items) != 1 || resp.Items[0].Title != "B" {
									t.Errorf("expected only B, got %+v", resp.Items)
								}
							},
						},
					},
					{
						step: "platform+status combined", method: "GET", path: "/topics?platform=抖音&status=已发布",
						want: want{
							code: http.StatusOK,
							check: func(t *testing.T, b []byte, _ int, _ []models.Topic) {
								var resp struct {
									Items []models.Topic `json:"items"`
								}
								_ = json.Unmarshal(b, &resp)
								if len(resp.Items) != 1 || resp.Items[0].Title != "C" {
									t.Errorf("expected only C, got %+v", resp.Items)
								}
							},
						},
					},
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, db := setupTestRouter(t)
			for _, s := range tc.seeds {
				db.Create(&models.Topic{Title: s.title, Platform: s.platform, Status: s.status})
			}
			steps := tc.steps(t, r, nil)
			for _, st := range steps {
				t.Run(st.step, func(t *testing.T) {
					w := do(t, r, st.method, st.path, st.body)
					if w.Code != st.want.code {
						t.Fatalf("%s %s: expected %d, got %d, body: %s", st.method, st.path, st.want.code, w.Code, w.Body.String())
					}
					var all []models.Topic
					if w.Header().Get("Content-Type") == "application/json" {
						_ = json.Unmarshal(w.Body.Bytes(), &all) // optional
					}
					st.want.check(t, w.Body.Bytes(), w.Code, all)
				})
			}
		})
	}
}

func TestTopicCRUD_NegativeCases(t *testing.T) {
	cases := []struct {
		name          string
		method, path  string
		body          any
		wantCode      int
		wantErrSubstr string
	}{
		{
			name: "create with malformed JSON", method: "POST", path: "/topics",
			body:     map[string]any{"_": json.RawMessage(`{not json`)},
			wantCode: http.StatusBadRequest,
			// Use a raw body for the malformed case via custom helper below.
		},
		{
			name: "create missing required title", method: "POST", path: "/topics",
			body:          map[string]any{"platform": "抖音", "status": "想法"},
			wantCode:      http.StatusBadRequest,
			wantErrSubstr: "title",
		},
		{
			name: "create with unknown platform", method: "POST", path: "/topics",
			body:          map[string]any{"title": "x", "platform": "MySpace", "status": "想法"},
			wantCode:      http.StatusBadRequest,
			wantErrSubstr: "platform",
		},
		{
			name: "create with unknown status", method: "POST", path: "/topics",
			body:          map[string]any{"title": "x", "platform": "抖音", "status": "vaporware"},
			wantCode:      http.StatusBadRequest,
			wantErrSubstr: "status",
		},
		{
			name: "get with non-numeric id", method: "GET", path: "/topics/abc",
			wantCode:      http.StatusBadRequest,
			wantErrSubstr: "invalid id",
		},
		{
			name: "get with negative id", method: "GET", path: "/topics/-1",
			wantCode:      http.StatusBadRequest,
			wantErrSubstr: "invalid id",
		},
		{
			name: "get with zero id", method: "GET", path: "/topics/0",
			wantCode:      http.StatusBadRequest,
			wantErrSubstr: "invalid id",
		},
		{
			name: "get missing", method: "GET", path: "/topics/9999",
			wantCode:      http.StatusNotFound,
			wantErrSubstr: "not found",
		},
		{
			name: "update missing", method: "PATCH", path: "/topics/9999",
			body:          map[string]any{"title": "x"},
			wantCode:      http.StatusNotFound,
			wantErrSubstr: "not found",
		},
		{
			name: "update rejects bad status", method: "PATCH", path: "/topics/1",
			body:          map[string]any{"status": "garbage"},
			wantCode:      http.StatusBadRequest,
			wantErrSubstr: "status",
		},
		{
			name: "delete missing", method: "DELETE", path: "/topics/9999",
			wantCode:      http.StatusNotFound,
			wantErrSubstr: "not found",
		},
		{
			name: "list rejects unknown platform filter", method: "GET", path: "/topics?platform=garbage",
			wantCode:      http.StatusBadRequest,
			wantErrSubstr: "platform",
		},
		{
			name: "list rejects bad limit", method: "GET", path: "/topics?limit=-1",
			wantCode:      http.StatusBadRequest,
			wantErrSubstr: "limit",
		},
		{
			name: "list caps limit at max", method: "GET", path: fmt.Sprintf("/topics?limit=%d", maxListPageSize+1000),
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, db := setupTestRouter(t)
			// Seed a single topic so PATCH/DELETE-of-missing are distinct from
			// PATCH/DELETE on a real row.
			db.Create(&models.Topic{Title: "seed", Platform: "抖音", Status: "想法"})

			var w *httptest.ResponseRecorder
			if tc.name == "create with malformed JSON" {
				// Need raw bytes for the malformed case; build a custom request.
				req, _ := http.NewRequest(tc.method, tc.path, bytes.NewBufferString("{not json"))
				req.Header.Set("Content-Type", "application/json")
				w = httptest.NewRecorder()
				r.ServeHTTP(w, req)
			} else {
				w = do(t, r, tc.method, tc.path, tc.body)
			}

			if w.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d, body: %s", tc.wantCode, w.Code, w.Body.String())
			}
			if tc.wantErrSubstr != "" {
				var resp map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode: %v", err)
				}
				msg, _ := resp["error"].(string)
				if !contains(msg, tc.wantErrSubstr) {
					t.Errorf("error %q does not contain %q", msg, tc.wantErrSubstr)
				}
			}
		})
	}
}

// contains is a small case-insensitive substring check so we don't have to
// import strings just for this.
func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	hl, nl := []rune(haystack), []rune(needle)
	for i := 0; i+len(nl) <= len(hl); i++ {
		match := true
		for j := 0; j < len(nl); j++ {
			a, b := hl[i+j], nl[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Sanity: the new gormTopicStore adapter implements TopicStore.
var _ TopicStore = (*gormTopicStore)(nil)
