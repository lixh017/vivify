package handlers

import (
	"errors"
	"testing"
)

// TestRequireUserID enforces the Phase 2 Task 3 invariant: a zero
// userID passed to a gorm* store method must error out with
// ErrMissingUserID. This is the load-bearing guard against
// cross-tenant reads/writes when a future call site (admin tool,
// MCP, cron) forgets to stamp a user. The store-level test below
// is the only place we pin the contract; without it, a future
// "let's relax this for unit tests" PR would have nothing to
// catch the regression.
func TestRequireUserID(t *testing.T) {
	if err := requireUserID(0); !errors.Is(err, ErrMissingUserID) {
		t.Errorf("userID=0: got %v, want ErrMissingUserID", err)
	}
	if err := requireUserID(1); err != nil {
		t.Errorf("userID=1: got %v, want nil", err)
	}
	if err := requireUserID(42); err != nil {
		t.Errorf("userID=42: got %v, want nil", err)
	}
}
