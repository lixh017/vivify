package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
)

// videoOverrideFn is the shape of the test seam that
// replaces agents.MiniMax.Video in handler tests. Each
// test wires a MiniMax client via NewMiniMaxWithVideoOverride
// and can return either a successful MiniMaxVideoResult
// or an error to exercise the 502 / 503 / 504 paths.
type videoOverrideFn func(ctx context.Context, prompt string, opts agents.MiniMaxVideoOptions) (*agents.MiniMaxVideoResult, error)

func setupVideoTestRouter(t *testing.T, fn videoOverrideFn) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	m := agents.NewMiniMaxWithVideoOverride(fn)
	r := gin.New()
	NewVideoHandler(m).RegisterRoutes(r)
	return r
}

// TestVideoHappyPath: override returns a clean
// MiniMaxVideoResult and the handler must stamp the
// response shape with the provider label and the saved
// path. The cost stamp is wired via the call_log
// middleware; without the middleware the ref is nil and
// StampFlatCost is a no-op (the response still flows).
func TestVideoHappyPath(t *testing.T) {
	r := setupVideoTestRouter(t, func(_ context.Context, prompt string, opts agents.MiniMaxVideoOptions) (*agents.MiniMaxVideoResult, error) {
		if !strings.Contains(prompt, "panda") {
			t.Errorf("prompt passed to client = %q, want it to contain 'panda'", prompt)
		}
		if opts.OutPath == "" {
			t.Error("out_path option was not propagated to the client")
		}
		return &agents.MiniMaxVideoResult{Path: opts.OutPath}, nil
	})
	w := doJSON(t, r, http.MethodPost, "/video", map[string]any{
		"prompt": "a panda eating bamboo",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp videoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.Provider != "minimax" {
		t.Errorf("provider = %q, want minimax", resp.Provider)
	}
	if resp.Path == "" {
		t.Error("path empty — happy path should return a non-empty saved path")
	}
}

// TestVideoRejectsEmptyPrompt: the 400 path. The video
// client is never called.
func TestVideoRejectsEmptyPrompt(t *testing.T) {
	called := false
	r := setupVideoTestRouter(t, func(_ context.Context, _ string, _ agents.MiniMaxVideoOptions) (*agents.MiniMaxVideoResult, error) {
		called = true
		return nil, nil
	})
	w := doJSON(t, r, http.MethodPost, "/video", map[string]any{
		"prompt": "   ",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "prompt is required") {
		t.Errorf("body = %s, want 'prompt is required'", w.Body.String())
	}
	if called {
		t.Error("client.Video was called for empty prompt — validation must short-circuit")
	}
}

// TestVideoNoAPIKey: with no API key and no override the
// handler must return 503.
//
// We isolate the test by unsetting the env var + pointing
// the mmx binary at /bin/false. Otherwise a real mmx on the
// PATH would happily call out via its own config file
// (~/.mmx/config.json) and bypass the handler's
// "Available()" gate, defeating the test.
func TestVideoNoAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MINIMAX_API_KEY", "")
	m := agents.NewMiniMaxForDemo()
	// binary path no longer used (HTTP-based)
	r := gin.New()
	NewVideoHandler(m).RegisterRoutes(r)

	w := doJSON(t, r, http.MethodPost, "/video", map[string]any{
		"prompt": "test",
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "MINIMAX_API_KEY") {
		t.Errorf("body = %s, want it to mention MINIMAX_API_KEY", w.Body.String())
	}
}

// TestVideoProviderError: the override returns a
// non-auth-shaped error and the handler must surface it as
// 502.
func TestVideoProviderError(t *testing.T) {
	r := setupVideoTestRouter(t, func(_ context.Context, _ string, _ agents.MiniMaxVideoOptions) (*agents.MiniMaxVideoResult, error) {
		return nil, errors.New("mmx video generate: connection refused")
	})
	w := doJSON(t, r, http.MethodPost, "/video", map[string]any{
		"prompt": "test",
	})
	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502, body = %s", w.Code, w.Body.String())
	}
}

// TestVideoTimeout: when the context the handler passes
// into the client is cancelled with context.DeadlineExceeded
// the handler must surface that as 504. We simulate it by
// returning a deadline-exceeded error from the override
// (the handler uses errors.Is(err, context.DeadlineExceeded)
// to pick the 504 branch — that's the error the upstream
// poll loop also returns when the outer ctx fires).
func TestVideoTimeout(t *testing.T) {
	r := setupVideoTestRouter(t, func(_ context.Context, _ string, _ agents.MiniMaxVideoOptions) (*agents.MiniMaxVideoResult, error) {
		return nil, context.DeadlineExceeded
	})
	w := doJSON(t, r, http.MethodPost, "/video", map[string]any{
		"prompt": "test",
	})
	if w.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504, body = %s", w.Code, w.Body.String())
	}
}
