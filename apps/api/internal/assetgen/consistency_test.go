package assetgen_test

import (
	"testing"

	"github.com/opc/api/internal/assetgen"
)

// TestConsistencyCheckUnknownType confirms the dispatcher returns
// Score=0 + explicit fail message when given a Profile whose Type
// isn't registered.
func TestConsistencyCheckUnknownType(t *testing.T) {
	t.Parallel()

	bogus := &bogusProfile{typeName: "unicorn"}
	got := assetgen.ConsistencyCheck(bogus, "anything")

	if got.Score != 0 {
		t.Errorf("Score = %v, want 0", got.Score)
	}
	if len(got.Fails) != 1 {
		t.Fatalf("len(Fails) = %d, want 1 (Fails: %v)", len(got.Fails), got.Fails)
	}
	if got.Fails[0] != "unknown profile type: unicorn" {
		t.Errorf("Fails[0] = %q, want %q", got.Fails[0], "unknown profile type: unicorn")
	}
}

// TestConsistencyCheckNilProfile confirms safeCheck guards nil.
func TestConsistencyCheckNilProfile(t *testing.T) {
	t.Parallel()

	got := assetgen.ConsistencyCheck(nil, "anything")
	if got.Score != 0 {
		t.Errorf("Score = %v, want 0", got.Score)
	}
	if len(got.Fails) != 1 || got.Fails[0] != "nil profile" {
		t.Errorf("Fails = %v, want [\"nil profile\"]", got.Fails)
	}
}

// bogusProfile implements assetgen.Profile but Type() returns a
// value never registered, so the dispatcher will hit the
// unknown-type branch.
type bogusProfile struct{ typeName string }

func (b *bogusProfile) Type() string    { return b.typeName }
func (b *bogusProfile) Version() string { return "x" }
func (b *bogusProfile) Name() string    { return "x" }
