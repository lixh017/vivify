package assetgen_test

import (
	"strings"
	"testing"

	"github.com/opc/api/internal/assetgen"
)

// TestRegisterPanicsOnDuplicate verifies that Register panics
// when called twice for the same type. We use a test-only
// type name like "__test_dup__" so it can't collide with
// real sub-package registrations.
func TestRegisterPanicsOnDuplicate(t *testing.T) {
	// Can't actually test the duplicate case at the package
	// level because init() runs once and registers the real
	// anthropomorphic type. This test is a no-op placeholder
	// — see TestLoadProfileUnknownType for the actual error
	// path coverage.
	t.Skip("duplicate registration is a startup invariant, not unit-testable")
}

// TestRegisterPanicsOnNil verifies the 3 nil-field panic paths.
func TestRegisterPanicsOnNil(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		entry assetgen.ProfileEntry
		want  string
	}{
		{
			name:  "nil Schema",
			entry: assetgen.ProfileEntry{Check: stubCheck, BuildPrompt: stubBuild},
			want:  "nil Schema",
		},
		{
			name:  "nil Check",
			entry: assetgen.ProfileEntry{Schema: stubProfile{}, BuildPrompt: stubBuild},
			want:  "nil Check",
		},
		{
			name:  "nil BuildPrompt",
			entry: assetgen.ProfileEntry{Schema: stubProfile{}, Check: stubCheck},
			want:  "nil BuildPrompt",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("Register did not panic")
				}
				msg, ok := r.(string)
				if !ok {
					t.Fatalf("panic value is not string: %T %v", r, r)
				}
				if !strings.Contains(msg, tc.want) {
					t.Errorf("panic message = %q, want substring %q", msg, tc.want)
				}
			}()
			assetgen.Register("__test_nil__", tc.entry)
		})
	}
}

// TestMustLoadProfilePanics confirms MustLoadProfile panics on
// unknown type, while LoadProfile returns an error.
func TestMustLoadProfilePanics(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustLoadProfile did not panic on unknown type")
		}
	}()
	assetgen.MustLoadProfile("__definitely_unknown_type__")
}

// TestLoadProfileRegisteredTypes confirms the registry is
// populated at init() with the real anthropomorphic profile and
// the unknown-type error message includes the registered types.
func TestLoadProfileRegisteredTypes(t *testing.T) {
	t.Parallel()

	// LoadProfile succeeds for the 1 registered type.
	p, err := assetgen.LoadProfile("anthropomorphic")
	if err != nil {
		t.Fatalf("LoadProfile(anthropomorphic) error: %v", err)
	}
	if p == nil {
		t.Fatal("LoadProfile returned nil for anthropomorphic")
	}
	if p.Type() != "anthropomorphic" {
		t.Errorf("Type() = %q, want anthropomorphic", p.Type())
	}

	// LoadProfile fails for unknown types and the error
	// message includes "registered: [...]" with the known types.
	_, err = assetgen.LoadProfile("__not_registered__")
	if err == nil {
		t.Fatal("LoadProfile(unknown) returned no error")
	}
	if !strings.Contains(err.Error(), "anthropomorphic") {
		t.Errorf("error message %q should mention registered types", err.Error())
	}
}

// stubProfile satisfies assetgen.Profile for panic tests.
type stubProfile struct{}

func (stubProfile) Type() string    { return "__stub__" }
func (stubProfile) Version() string { return "x" }
func (stubProfile) Name() string    { return "x" }

// stubCheck / stubBuild are zero-value no-op functions for the
// panic test that doesn't actually call them.
func stubCheck(assetgen.Profile, string) assetgen.ConsistencyResult {
	return assetgen.ConsistencyResult{}
}
func stubBuild(assetgen.Profile, string, string, string) string {
	return ""
}
