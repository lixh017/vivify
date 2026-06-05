package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBuildCoverPromptDefaults covers the case where the
// caller omits style + platform. The defaults are baked into
// the prompt because the providers do not have a meaningful
// fallback when given a half-empty prompt.
func TestBuildCoverPromptDefaults(t *testing.T) {
	p := buildCoverPrompt(CoverRequest{Title: "凌晨 3 点熊猫不睡"})
	for _, want := range []string{"治愈系", "抖音", "9:16", "凌晨 3 点熊猫不睡"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q: %s", want, p)
		}
	}
}

// TestBuildCoverPromptPlatformRatio verifies the 哔哩哔哩
// branch flips the aspect ratio to 16:9 widescreen. The
// assertion is intentionally on the substring so a future
// tweak to the prompt text does not break this contract —
// what matters is that 哔哩哔哩 callers get a widescreen hint.
func TestBuildCoverPromptPlatformRatio(t *testing.T) {
	p := buildCoverPrompt(CoverRequest{Title: "庄子", Platform: "哔哩哔哩"})
	if !strings.Contains(p, "16:9") {
		t.Errorf("expected 16:9 in prompt, got %s", p)
	}
	if strings.Contains(p, "9:16") {
		t.Errorf("expected NO 9:16 in 哔哩哔哩 prompt, got %s", p)
	}
}

// TestCoverClientNoKeyReturnsMock asserts the dev-mode
// contract: an empty API key never hits the network. The
// returned result is a data: URL with a small SVG, and the
// provider label is "mock" so the frontend can render a
// "this is a placeholder" badge.
func TestCoverClientNoKeyReturnsMock(t *testing.T) {
	c := NewCoverClient("", "auto")
	if c.Available() {
		t.Errorf("Available() = true with empty key, want false")
	}
	res, err := c.Generate(context.Background(), CoverRequest{Title: "x"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Provider != CoverProviderMock {
		t.Errorf("provider = %q, want %q", res.Provider, CoverProviderMock)
	}
	if !strings.HasPrefix(res.ImageURL, "data:image/svg+xml;base64,") {
		t.Errorf("expected data: URL, got %q", res.ImageURL[:min(60, len(res.ImageURL))])
	}
	if res.PromptUsed == "" {
		t.Errorf("prompt_used is empty")
	}
	if res.GenerationTimeMs < 0 {
		t.Errorf("generation_time_ms is negative: %d", res.GenerationTimeMs)
	}
}

// TestCoverClientMockProviderSkipsNetwork ensures the
// explicit "mock" provider branch never even constructs a
// transport. We test this by giving the client a junk api
// key and asserting the request goes to the data: URL branch
// (no panic, no error).
func TestCoverClientMockProviderSkipsNetwork(t *testing.T) {
	c := NewCoverClient("would-be-ignored", CoverProviderMock)
	if !c.Available() {
		t.Errorf("Available() = false with mock provider, want true")
	}
	res, err := c.Generate(context.Background(), CoverRequest{Title: "y"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Provider != CoverProviderMock {
		t.Errorf("provider = %q, want mock", res.Provider)
	}
}

// TestCoverClientKlingHappyPath stands up a fake
// 火山引擎-style endpoint, points the client at it via
// volcengineBaseURL, and asserts the JSON body is parsed
// into the image_urls[0] field. The endpoint URL is
// overridden via the package-level var so we do not need a
// DI refactor.
func TestCoverClientKlingHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/kling/image_generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"image_urls":["https://cdn.example.com/cover.png"]}}`))
	}))
	defer srv.Close()

	orig := volcengineBaseURL
	volcengineBaseURL = srv.URL
	defer func() { volcengineBaseURL = orig }()

	c := NewCoverClient("test-key", "kling")
	res, err := c.Generate(context.Background(), CoverRequest{Title: "ok"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.ImageURL != "https://cdn.example.com/cover.png" {
		t.Errorf("image_url = %q, want cdn URL", res.ImageURL)
	}
	if res.Provider != CoverProviderKling {
		t.Errorf("provider = %q, want kling", res.Provider)
	}
}

// TestCoverClientJimengPathAndShape checks the 即梦 branch:
// different URL path, different response shape
// (data.image_url, not data.image_urls[]). A regression
// here would surface as a 502 for Jimeng users.
func TestCoverClientJimengPathAndShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/jimeng/image_generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"image_url":"https://cdn.example.com/j.png"}}`))
	}))
	defer srv.Close()

	orig := volcengineBaseURL
	volcengineBaseURL = srv.URL
	defer func() { volcengineBaseURL = orig }()

	c := NewCoverClient("test-key", "jimeng")
	res, err := c.Generate(context.Background(), CoverRequest{Title: "ok"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.ImageURL != "https://cdn.example.com/j.png" {
		t.Errorf("image_url = %q", res.ImageURL)
	}
	if res.Provider != CoverProviderJimeng {
		t.Errorf("provider = %q, want jimeng", res.Provider)
	}
}

// TestCoverClientProviderErrorBubblesUp asserts that a 4xx
// from the provider surfaces as a Go error (not a silent
// fallback to mock). A mock fallback in this branch would
// let the operator think they hit a real provider when
// they did not — the failure has to be visible.
func TestCoverClientProviderErrorBubblesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"invalid access key"}`))
	}))
	defer srv.Close()
	orig := volcengineBaseURL
	volcengineBaseURL = srv.URL
	defer func() { volcengineBaseURL = orig }()

	c := NewCoverClient("bad", "kling")
	_, err := c.Generate(context.Background(), CoverRequest{Title: "x"})
	if err == nil {
		t.Fatalf("expected error on 401, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got %v", err)
	}
}

// TestMockCoverImageURLDeterministic asserts that two calls
// for the same input produce the same digest (and therefore
// the same SVG hash suffix). This is what makes the
// placeholder a stable cache key in the demo path.
func TestMockCoverImageURLDeterministic(t *testing.T) {
	a := mockCoverImageURL(CoverRequest{Title: "x", Style: "治愈", Platform: "抖音"})
	b := mockCoverImageURL(CoverRequest{Title: "x", Style: "治愈", Platform: "抖音"})
	if a != b {
		t.Errorf("expected identical placeholder, got different SVGs")
	}
	c := mockCoverImageURL(CoverRequest{Title: "y"})
	if a == c {
		t.Errorf("expected different placeholder for different title")
	}
}

// TestEscapeXMLEscapesTitle ensures user-supplied titles
// cannot inject SVG markup. The check focuses on the most
// common injection vector (<, >, &) — quote escaping is
// covered indirectly because we put the title inside an
// attribute only when wrapped in &quot; etc.
func TestEscapeXMLEscapesTitle(t *testing.T) {
	got := escapeXML(`<script>alert("x")</script> & '`)
	for _, bad := range []string{"<script>", "'", `"x"`} {
		if strings.Contains(got, bad) {
			t.Errorf("unescaped char %q in %q", bad, got)
		}
	}
}

// TestProviderNameResolvesAuto covers the "auto" branch:
// no explicit provider, key set → "kling" (canonical
// default), no key set → "mock".
func TestProviderNameResolvesAuto(t *testing.T) {
	withKey := NewCoverClient("k", "auto")
	if got := withKey.ProviderName(); got != CoverProviderKling {
		t.Errorf("with key, provider = %q, want kling", got)
	}
	noKey := NewCoverClient("", "auto")
	if got := noKey.ProviderName(); got != CoverProviderMock {
		t.Errorf("no key, provider = %q, want mock", got)
	}
}

// min is a tiny helper to keep the import list small. (Go
// 1.21+ has a builtin but we keep the explicit form to stay
// readable for reviewers on older toolchains.)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
