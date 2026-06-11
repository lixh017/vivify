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

// speechOverrideFn is the shape of the test seam that
// replaces agents.MiniMax.Speech in handler tests. Each test
// wires a MiniMax client via NewMiniMaxWithSpeechOverride
// and can return either a successful MiniMaxSpeechResult
// or an error to exercise the 502 / 503 paths.
type speechOverrideFn func(ctx context.Context, text string, opts agents.MiniMaxSpeechOptions) (*agents.MiniMaxSpeechResult, error)

func setupSpeechTestRouter(t *testing.T, fn speechOverrideFn) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	m := agents.NewMiniMaxWithSpeechOverride(fn)
	r := gin.New()
	NewSpeechHandler(m).RegisterRoutes(r)
	return r
}

// TestSpeechHappyPath: override returns a clean
// MiniMaxSpeechResult and the handler must stamp the
// response shape with the provider label and the saved
// path. The cost stamp is wired via the call_log
// middleware; without the middleware the ref is nil and
// StampFlatCost is a no-op (the response still flows).
func TestSpeechHappyPath(t *testing.T) {
	r := setupSpeechTestRouter(t, func(_ context.Context, text string, opts agents.MiniMaxSpeechOptions) (*agents.MiniMaxSpeechResult, error) {
		// Echo the input the handler passed so the test
		// can assert the contract.
		if !strings.Contains(text, "雨") {
			t.Errorf("text passed to client = %q, want it to contain '雨'", text)
		}
		if opts.Voice == "" {
			t.Error("voice option was not propagated to the client")
		}
		return &agents.MiniMaxSpeechResult{
			Path:       opts.OutPath,
			DurationMs: 1234,
			SizeBytes:  5678,
			SampleRate: 24000,
		}, nil
	})
	w := doJSON(t, r, http.MethodPost, "/speech", map[string]any{
		"text":  "今天的雨",
		"voice": "Chinese_mandarin_warm",
		"speed": 1.0,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", w.Code, w.Body.String())
	}
	var resp speechResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	if resp.Provider != "minimax" {
		t.Errorf("provider = %q, want minimax", resp.Provider)
	}
	if resp.Path == "" {
		t.Error("path empty — happy path should return a non-empty saved path")
	}
	if resp.DurationMs != 1234 {
		t.Errorf("duration_ms = %d, want 1234", resp.DurationMs)
	}
	if resp.SampleRate != 24000 {
		t.Errorf("sample_rate = %d, want 24000", resp.SampleRate)
	}
}

// TestSpeechRejectsEmptyText: the 400 path. The image
// client is never called.
func TestSpeechRejectsEmptyText(t *testing.T) {
	called := false
	r := setupSpeechTestRouter(t, func(_ context.Context, _ string, _ agents.MiniMaxSpeechOptions) (*agents.MiniMaxSpeechResult, error) {
		called = true
		return nil, nil
	})
	w := doJSON(t, r, http.MethodPost, "/speech", map[string]any{
		"text": "   ",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "text is required") {
		t.Errorf("body = %s, want 'text is required'", w.Body.String())
	}
	if called {
		t.Error("client.Speech was called for empty text — validation must short-circuit")
	}
}

// TestSpeechNoAPIKey: with no API key and no override the
// handler must return 503. We don't want a 200 here — the
// speech path is a real binary output with no canned demo
// fallback, so "no key" should surface as a hard
// unavailability rather than a fake success.
//
// We isolate the test by unsetting the env var + pointing
// the mmx binary at /bin/false. Otherwise a real mmx on the
// PATH would happily call out via its own config file
// (~/.mmx/config.json) and bypass the handler's
// "Available()" gate, defeating the test.
func TestSpeechNoAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MINIMAX_API_KEY", "")
	m := agents.NewMiniMaxForDemo()
	// binary path no longer used (HTTP-based)
	r := gin.New()
	NewSpeechHandler(m).RegisterRoutes(r)

	w := doJSON(t, r, http.MethodPost, "/speech", map[string]any{
		"text": "test",
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "MINIMAX_API_KEY") {
		t.Errorf("body = %s, want it to mention MINIMAX_API_KEY", w.Body.String())
	}
}

// TestSpeechAPIKeyError: the override returns an
// API-key-shaped error and the handler must surface it as
// 503 (not 502) so the operator dashboard can tell "not
// configured" apart from "provider failed".
func TestSpeechAPIKeyError(t *testing.T) {
	r := setupSpeechTestRouter(t, func(_ context.Context, _ string, _ agents.MiniMaxSpeechOptions) (*agents.MiniMaxSpeechResult, error) {
		return nil, errors.New("minimax_api_key: unauthorized 401")
	})
	w := doJSON(t, r, http.MethodPost, "/speech", map[string]any{
		"text": "test",
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503, body = %s", w.Code, w.Body.String())
	}
}

// TestSpeechProviderError: the override returns a
// non-auth-shaped error and the handler must surface it as
// 502.
func TestSpeechProviderError(t *testing.T) {
	r := setupSpeechTestRouter(t, func(_ context.Context, _ string, _ agents.MiniMaxSpeechOptions) (*agents.MiniMaxSpeechResult, error) {
		return nil, errors.New("mmx speech synthesize: unexpected EOF")
	})
	w := doJSON(t, r, http.MethodPost, "/speech", map[string]any{
		"text": "test",
	})
	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502, body = %s", w.Code, w.Body.String())
	}
}
