package handlers

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/opc/api/internal/agents"
)

// stubResolverEcho returns agents.Echo for any (userID, scope).
// It mirrors the production resolver's "no credential configured"
// path so the helper's fall-through-to-default contract is
// testable without a real gorm.DB.
type stubResolverEcho struct{}

func (stubResolverEcho) Text(_ context.Context, _ uint, _ string) (agents.TextProvider, error) {
	return agents.Echo, nil
}

// newAuthedCtx returns a *gin.Context with user_id=42 set and a
// non-nil *http.Request so the helper can call c.Request.Context()
// without panicking. httptest.NewRecorder() alone leaves Request
// nil, which is the test panic the original draft hit.
func newAuthedCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set("user_id", uint(42))
	return c
}

func newUnauthedCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	return c
}

func TestResolveTextForRequest_FallsThroughEchoToDefault(t *testing.T) {
	// Contract (Phase 4 smoke regression, 2026-06-16): when the
	// resolver returns Echo — the "no credential configured"
	// sentinel — the helper must fall through to the configured
	// defaultText so shouldUseDemo() can short-circuit to the
	// canned demo pool (which expects a real provider whose
	// Available() reports false when its API key is missing).
	//
	// Pre-fix, the helper returned Echo directly, which made
	// shouldUseDemo() return false (Echo.Available()==true) and
	// routed the call into the live path; Echo's body is
	// "[echo:echo] <prompt>", which then failed JSON parsing
	// with "invalid character 'e' looking for beginning of
	// value" → 503.
	c := newAuthedCtx()
	defaultProvider := agents.NewMiniMax("") // Available()==false when MINIMAX_API_KEY is empty
	p := resolveTextForRequest(c, stubResolverEcho{}, defaultProvider)
	if p == agents.Echo {
		t.Fatalf("helper returned Echo despite default being non-nil: shouldUseDemo cannot short-circuit")
	}
	if p != defaultProvider {
		t.Errorf("helper returned %q, want default provider", p.Name())
	}
}

func TestResolveTextForRequest_ResolvesRealProvider(t *testing.T) {
	// When the resolver returns a real (non-Echo) provider, the
	// helper must return it directly — no fall-through. This is
	// the happy path for a user with a configured credential.
	c := newAuthedCtx()
	realProvider := agents.NewMiniMaxWithTextOverride(func(_ context.Context, _ string, _ agents.MiniMaxTextOptions) (*agents.MiniMaxTextResult, error) {
		return &agents.MiniMaxTextResult{Text: "real"}, nil
	})
	resolver := stubReturning{realProvider}

	defaultProvider := agents.NewMiniMax("") // Available()==false
	p := resolveTextForRequest(c, resolver, defaultProvider)
	if p != realProvider {
		t.Errorf("helper returned %q, want real provider %q", p.Name(), realProvider.Name())
	}
}

func TestResolveTextForRequest_NilDefaultReturnsEcho(t *testing.T) {
	// Safety net: if both the resolver returns Echo AND the
	// default is nil, the helper returns the package-level Echo
	// to guarantee a non-nil provider. shouldUseDemo() will
	// then return false (Echo.Available()==true), but at least
	// the call site never panics on a nil deref.
	c := newAuthedCtx()
	p := resolveTextForRequest(c, stubResolverEcho{}, nil)
	if p == nil {
		t.Fatal("helper returned nil")
	}
	if p != agents.Echo {
		t.Errorf("helper returned %q, want agents.Echo", p.Name())
	}
}

func TestResolveTextForRequest_NilResolverUsesDefault(t *testing.T) {
	// When the resolver itself is nil (legacy construction path
	// or test setup), the helper goes straight to defaultText.
	c := newAuthedCtx()
	defaultProvider := agents.NewMiniMax("")
	p := resolveTextForRequest(c, nil, defaultProvider)
	if p != defaultProvider {
		t.Errorf("helper returned %q, want default %q", p.Name(), defaultProvider.Name())
	}
}

func TestResolveTextForRequest_UserIDZeroBypassesResolver(t *testing.T) {
	// Background paths (no per-request user) must NOT consult
	// the resolver. The resolver short-circuits to Echo on
	// userID==0 in production, so the helper mirrors that gate
	// to avoid pinging the DB or — worse — returning Echo and
	// confusing shouldUseDemo.
	c := newUnauthedCtx()
	defaultProvider := agents.NewMiniMax("")
	resolver := stubReturning{agents.Echo} // would be returned if helper called it
	p := resolveTextForRequest(c, resolver, defaultProvider)
	if p == agents.Echo {
		t.Errorf("helper consulted resolver despite userID=0: should bypass and use default")
	}
	if p != defaultProvider {
		t.Errorf("helper returned %q, want default %q", p.Name(), defaultProvider.Name())
	}
}

// stubReturning is a resolver shim that always returns the
// configured provider (for happy-path tests).
type stubReturning struct {
	p agents.TextProvider
}

func (s stubReturning) Text(_ context.Context, _ uint, _ string) (agents.TextProvider, error) {
	return s.p, nil
}
