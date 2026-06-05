package handlers

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
)

// fakeCoverGen is the in-memory CoverGenerator the handler
// tests inject. It returns whatever the test configured
// (response + error) and records the request shape for
// later assertions.
type fakeCoverGen struct {
	mu        atomic.Int32
	available bool
	resp      agents.CoverResult
	err       error
	lastReq   agents.CoverRequest
}

func (f *fakeCoverGen) Generate(_ context.Context, req agents.CoverRequest) (agents.CoverResult, error) {
	f.mu.Add(1)
	f.lastReq = req
	return f.resp, f.err
}

func (f *fakeCoverGen) Available() bool { return f.available }

// setupCoverRouter wires a CoverHandler with a fake
// generator. The generator is configured by the caller
// before passing it in.
func setupCoverRouter(t *testing.T, gen agents.CoverGenerator) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewCoverHandler(gen).RegisterRoutes(r)
	return r
}

// TestCoverSuccessMockProvider asserts the happy path
// against the mock provider. The response shape is what
// the frontend <img>-renders: image_url, prompt_used,
// provider, generation_time_ms.
func TestCoverSuccessMockProvider(t *testing.T) {
	gen := &fakeCoverGen{
		available: true,
		resp: agents.CoverResult{
			ImageURL:         "data:image/svg+xml;base64,abc",
			PromptUsed:       "治愈系 风格封面,主体:x,平台:抖音,画幅:vertical 9:16,高质量,精细插画",
			Provider:         "mock",
			GenerationTimeMs: 12,
		},
	}
	r := setupCoverRouter(t, gen)
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
		`"image_url":"data:image/svg+xml;base64,abc"`,
		`"provider":"mock"`,
		`"generation_time_ms":12`,
		`"prompt_used":"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
	if gen.lastReq.Title != "x" {
		t.Errorf("title not propagated: %q", gen.lastReq.Title)
	}
	if gen.lastReq.Style != "治愈" {
		t.Errorf("style not propagated: %q", gen.lastReq.Style)
	}
}

// TestCoverRejectsEmptyTitle asserts the 400 path for the
// most common validation error.
func TestCoverRejectsEmptyTitle(t *testing.T) {
	gen := &fakeCoverGen{available: true}
	r := setupCoverRouter(t, gen)
	w := doJSON(t, r, http.MethodPost, "/ai/cover", map[string]any{
		"title": "   ",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "title is required") {
		t.Errorf("body = %s, want 'title is required'", w.Body.String())
	}
	if gen.mu.Load() != 0 {
		t.Errorf("generator called %d times on validation failure, want 0", gen.mu.Load())
	}
}

// TestCoverReturnsBadGatewayOnProviderError asserts the
// 502 path: the generator returned an error (e.g. real
// provider returned 401). The handler must surface the
// error message and include the prompt that was sent so an
// operator can debug without re-running with curl.
func TestCoverReturnsBadGatewayOnProviderError(t *testing.T) {
	gen := &fakeCoverGen{
		available: true,
		resp: agents.CoverResult{
			PromptUsed: "the prompt that was sent",
		},
		err: &genError{msg: "provider kling returned 401: invalid access key"},
	}
	r := setupCoverRouter(t, gen)
	w := doJSON(t, r, http.MethodPost, "/ai/cover", map[string]any{
		"title": "y",
	})
	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "401") {
		t.Errorf("body = %s, want 401 in error", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "the prompt that was sent") {
		t.Errorf("body = %s, want prompt echoed", w.Body.String())
	}
}

// TestCoverWithRealCoverClientMock is the end-to-end
// smoke test against the production CoverClient in mock
// mode. The point is to assert the data: URL is rendered
// verbatim and provider == "mock" comes back, so a
// frontend integration can be tested without any external
// services.
func TestCoverWithRealCoverClientMock(t *testing.T) {
	c := agents.NewCoverClient("", "mock")
	r := setupCoverRouter(t, c)
	w := doJSON(t, r, http.MethodPost, "/ai/cover", map[string]any{
		"title": "panda 凌晨 3 点",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"provider":"mock"`) {
		t.Errorf("body = %s, want provider=mock", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "data:image/svg+xml;base64,") {
		t.Errorf("body = %s, want data: URL", w.Body.String())
	}
}

// TestCoverWithRealCoverClientAutoNoKey asserts the
// "auto provider, no key" branch: Available() returns false
// (so the handler does not warn) but Generate() still
// returns a mock image, and provider is "mock" in the
// response.
func TestCoverWithRealCoverClientAutoNoKey(t *testing.T) {
	c := agents.NewCoverClient("", "auto")
	if c.Available() {
		t.Errorf("Available() with empty key + auto = true, want false")
	}
	r := setupCoverRouter(t, c)
	w := doJSON(t, r, http.MethodPost, "/ai/cover", map[string]any{
		"title": "any",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"provider":"mock"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

// TestCoverTrimsStringFields asserts the handler trims
// leading / trailing whitespace from the request fields
// before passing them to the generator. The generator
// relies on trimmed values to pick the correct default
// style / platform.
func TestCoverTrimsStringFields(t *testing.T) {
	gen := &fakeCoverGen{
		available: true,
		resp:      agents.CoverResult{ImageURL: "x", Provider: "mock"},
	}
	r := setupCoverRouter(t, gen)
	w := doJSON(t, r, http.MethodPost, "/ai/cover", map[string]any{
		"title":    "  panda  ",
		"style":    "  治愈  ",
		"platform": "  抖音  ",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := gen.lastReq.Title; got != "panda" {
		t.Errorf("title = %q, want 'panda'", got)
	}
	if got := gen.lastReq.Style; got != "治愈" {
		t.Errorf("style = %q, want '治愈'", got)
	}
	if got := gen.lastReq.Platform; got != "抖音" {
		t.Errorf("platform = %q, want '抖音'", got)
	}
}

// TestCoverPropagatesTopicID is a small regression test
// for the persistence seam: topic_id is forwarded as-is so
// the future auto-persist branch can use it.
func TestCoverPropagatesTopicID(t *testing.T) {
	gen := &fakeCoverGen{
		available: true,
		resp:      agents.CoverResult{ImageURL: "x", Provider: "mock"},
	}
	r := setupCoverRouter(t, gen)
	w := doJSON(t, r, http.MethodPost, "/ai/cover", map[string]any{
		"title":    "t",
		"topic_id": 42,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := gen.lastReq.TopicID; got != 42 {
		t.Errorf("topic_id = %d, want 42", got)
	}
}

// TestCoverWithRealClientKlingProviderResolves is the
// final integration test: the production CoverClient with
// a real-looking key (no actual network call) and a "kling"
// provider must report provider="kling" in the
// ProviderName() path, so a future wiring change that
// drops the provider label would surface here.
func TestCoverWithRealClientKlingProviderResolves(t *testing.T) {
	c := agents.NewCoverClient("test-key", "kling")
	if !c.Available() {
		t.Errorf("Available() = false with key, want true")
	}
	if c.ProviderName() != "kling" {
		t.Errorf("ProviderName() = %q, want kling", c.ProviderName())
	}
	// We deliberately do not call Generate() because that
	// would hit the live 火山引擎 endpoint. The handler
	// integration is covered by TestCoverWithRealCoverClientMock.
	_ = time.Now() // keep the time import in case of future timing tests
}

// genError is a tiny error type so the test file can
// control the exact wording of the 502 body. We avoid
// fmt.Errorf to keep the test free of incidental string
// formatting in case a future change normalizes error
// messages.
type genError struct{ msg string }

func (e *genError) Error() string { return e.msg }
