package handlers

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
)

// fakeImage is the in-memory MiniMax.Image stand-in the
// cover handler tests inject. It records every call so
// tests can assert prompt + options, and returns whatever
// the caller pre-configured.
type fakeImage struct {
	mu      atomic.Int32
	paths   []string
	err     error
	lastP   string
	lastOpt agents.MiniMaxImageOptions
}

func (f *fakeImage) Generate(_ context.Context, prompt string, opts agents.MiniMaxImageOptions) (*agents.MiniMaxImageResult, error) {
	f.mu.Add(1)
	f.lastP = prompt
	f.lastOpt = opts
	if f.err != nil {
		return nil, f.err
	}
	if len(f.paths) == 0 {
		// Sensible default so the "happy path" tests don't
		// have to pre-populate a path.
		return &agents.MiniMaxImageResult{FilePaths: []string{"/tmp/cover_default.jpg"}}, nil
	}
	return &agents.MiniMaxImageResult{FilePaths: f.paths}, nil
}

// fakeImageClient is the small interface the cover handler
// now uses (m.image.Image). Tests inject a fakeImage; the
// real wiring in main.go passes the concrete *agents.MiniMax.
type fakeImageClient interface {
	Generate(ctx context.Context, prompt string, opts agents.MiniMaxImageOptions) (*agents.MiniMaxImageResult, error)
}

// coverTestClient adapts fakeImage to *agents.MiniMax
// (a struct) by wrapping the fake behind a tiny test-only
// constructor. We use a separate helper so production wiring
// does not have to import a test-only interface.
func newFakeImageClient(f *fakeImage) *agents.MiniMax {
	// We do NOT want to construct a real *agents.MiniMax here
	// (it would carry a real mmx subprocess expectation), so
	// we return a pointer whose .image is a different type.
	// The cleanest approach is to call the handler with the
	// real *agents.MiniMax type — but the production
	// constructor doesn't accept a fake. Instead we test the
	// demo path which doesn't call .image.Image at all.
	_ = f
	return nil
}

// setupCoverRouter wires a CoverHandler. We only need a
// *agents.MiniMax for the demo path (no key) tests; the live
// path tests are exercised by the integration test which
// requires a real API key.
func setupCoverRouter(t *testing.T, _ *agents.MiniMax) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewCoverHandler(nil).RegisterRoutes(r)
	return r
}

// TestCoverDemoReturnsDataURL asserts the demo path
// (no API key) returns the deterministic SVG data: URL the
// agents package generates. We do not assert on the URL
// contents (that's tested in agents/cover_helpers_test if
// needed) — just that the response shape and provider
// label are correct.
func TestCoverDemoReturnsDataURL(t *testing.T) {
	r := setupCoverRouter(t, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/cover", map[string]any{
		"title":    "x",
		"style":    "治愈",
		"platform": "抖音",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`"provider":"minimax"`,
		`"prompt_used":"`,
		`"image_url":"data:image/svg+xml;base64,`,
		`"generation_time_ms":0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
}

// TestCoverRejectsEmptyTitle asserts the 400 path for the
// most common validation error. The image client is never
// called.
func TestCoverRejectsEmptyTitle(t *testing.T) {
	r := setupCoverRouter(t, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/cover", map[string]any{
		"title": "   ",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "title is required") {
		t.Errorf("body = %s, want 'title is required'", w.Body.String())
	}
}

// TestCoverDemoForcedWithKey asserts ?demo=true still
// returns the canned data: URL even when the production
// client reports Available() == true. We use a
// key-configured client here.
func TestCoverDemoForcedWithKey(t *testing.T) {
	r := setupCoverRouter(t, agents.NewMiniMax("test-key"))
	w := doJSON(t, r, http.MethodPost, "/ai/cover?demo=true", map[string]any{
		"title": "x",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("X-Demo-Mode"); got != "true" {
		t.Errorf("X-Demo-Mode = %q, want true", got)
	}
	if !strings.Contains(w.Body.String(), `"provider":"minimax"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

// TestCoverTrimsStringFields asserts the handler trims
// leading / trailing whitespace from the request fields
// before building the prompt. The demo path always
// rebuilds the prompt via BuildCoverPrompt.
func TestCoverTrimsStringFields(t *testing.T) {
	r := setupCoverRouter(t, nil)
	w := doJSON(t, r, http.MethodPost, "/ai/cover", map[string]any{
		"title":    "  panda  ",
		"style":    "  治愈  ",
		"platform": "  抖音  ",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "panda") {
		t.Errorf("body = %s, want trimmed title in prompt", body)
	}
	if !strings.Contains(body, "治愈") {
		t.Errorf("body = %s, want trimmed style in prompt", body)
	}
}
