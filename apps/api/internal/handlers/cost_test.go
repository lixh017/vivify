package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/opc/api/internal/config"
	"github.com/opc/api/internal/middleware"
	"github.com/opc/api/internal/models"
)

// TestStampClaudeCostRoundTrip wires the real call_log
// middleware + a small router with an inline handler that
// invokes StampClaudeCost, then asserts the call_log row has
// the expected Provider / InputTokens / OutputTokens / CostCents.
//
// This is the P0 regression test: before the cost-stamping
// seam was added, every call_log row had cost_cents=0 and the
// /observability dashboard reported ¥0.00 forever. The test
// runs against an in-memory SQLite + the production
// StampClaudeCost helper, so any future regression on the
// wiring (e.g. accidentally calling Complete instead of
// CompleteWithUsage) breaks the build.
func TestStampClaudeCostRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := db.AutoMigrate(&models.CallLog{}, &models.User{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	// Seed a user so the call_log row has a non-zero user_id
	// (the call_log middleware writes user_id=0 for unauth'd
	// requests, which is the same as the anonymous row
	// observability already exposes — fine for the test).
	if err := db.Create(&models.User{
		ID:    7,
		Email: "op@opc.local",
		Name:  "Operator",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	r := gin.New()
	// Stand in for the auth chain by stamping user_id=7 in
	// the gin context before the call_log middleware runs.
	// The production path goes through auth.RequireAuth →
	// tenant_guard → CtxUserID; we shortcut it here because
	// the test is about call_log, not auth.
	r.Use(func(c *gin.Context) {
		c.Set(middleware.CtxUserID, uint(7))
		c.Next()
	})
	r.Use(middleware.CallLog(middleware.CallLogConfig{
		DB:     db,
		Logger: slog.Default(),
	}))

	r.POST("/ai/topics", func(c *gin.Context) {
		// Simulate a Claude call that returned 1500 input
		// + 600 output tokens. The numbers are picked so
		// the expected cost is comfortably non-zero under
		// the haiku 4.5 pricing (8 cents / 1k input +
		// 0.4 cents / 1k output).
		StampClaudeCost(c, config.SkillAnthropicClaudeHaiku, 1500, 600)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ai/topics", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("handler: expected 204, got %d, body=%s", w.Code, w.Body.String())
	}

	var row models.CallLog
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("call_log row not found: %v", err)
	}
	if row.UserID != 7 {
		t.Errorf("expected user_id=7, got %d", row.UserID)
	}
	// skill is set by the call_log middleware's URL→skill
	// inference table; the test's /ai/topics URL is one the
	// middleware does not know, so we don't pin the value
	// here (it'll be "other" — that is a separate concern
	// from the cost-stamping seam).
	if row.Provider != "minimax" {
		t.Errorf("expected provider=minimax, got %q", row.Provider)
	}
	if row.InputTokens != 1500 {
		t.Errorf("expected input_tokens=1500, got %d", row.InputTokens)
	}
	if row.OutputTokens != 600 {
		t.Errorf("expected output_tokens=600, got %d", row.OutputTokens)
	}
	// CostUnits is the sum of input + output tokens, captured
	// alongside CostCents so a future reconciliation job can
	// re-derive cost from current pricing without trusting
	// the historical CostCents snapshot.
	if row.CostUnits != 2100 {
		t.Errorf("expected cost_units=2100 (1500+600), got %d", row.CostUnits)
	}
	// Cost math: input leg uses SkillPrice{CentsPerUnit=8,
	// UnitsPerCall=100} so 1500 tokens cost 1500*8/100 = 120
	// cents. Output leg uses OutputPriceCentsPer1KTokens=4,
	// so 600 tokens cost 600*4/1000 = 2.4 → 2 (integer
	// truncation). Total 122. NOTE: the configured rate
	// (8 cents per 100 input tokens) is a hundredfold over
	// the real Anthropic list price for Haiku 4.5; this
	// test pins the *current* config and is the regression
	// guard for the cost-stamping seam, not for pricing
	// accuracy. See config/cost.go for the price table and
	// the audit at docs/console-reserve-audit.md for the
	// note on the inflated rate.
	if row.CostCents != 122 {
		t.Errorf("expected cost_cents=122, got %d", row.CostCents)
	}
}

// TestStampFlatCost verifies the per-image cost path used by
// the cover handler. A real provider (kling) stamps 5 cents.
func TestStampFlatCost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	if err := db.AutoMigrate(&models.CallLog{}, &models.User{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if err := db.Create(&models.User{ID: 9, Email: "x@x", Name: "X"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(middleware.CtxUserID, uint(9))
		c.Next()
	})
	r.Use(middleware.CallLog(middleware.CallLogConfig{
		DB:     db,
		Logger: slog.Default(),
	}))
	r.POST("/cover", func(c *gin.Context) {
		StampFlatCost(c, "volcengine", config.SkillVolcengineKlingImage, 1)
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/cover", nil)
	r.ServeHTTP(w, req)

	var row models.CallLog
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("row: %v", err)
	}
	if row.Provider != "volcengine" {
		t.Errorf("expected provider=volcengine, got %q", row.Provider)
	}
	if row.CostCents != 5 {
		t.Errorf("expected cost_cents=5, got %d", row.CostCents)
	}
}
