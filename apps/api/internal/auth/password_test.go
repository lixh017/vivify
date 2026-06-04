package auth

import (
	"errors"
	"strings"
	"testing"
)

// TestHashPasswordRoundTrip exercises the canonical "hash then verify"
// loop. We try a small matrix of inputs to make sure HashPassword does
// not silently produce an unparseable hash and that VerifyPassword
// returns nil for the right plaintext and a non-nil error for any
// deviation. The bcrypt cost is DefaultCost (~10), so the test runs in
// tens of milliseconds.
func TestHashPasswordRoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		plain string
	}{
		{"simple", "hunter2"},
		{"unicode", "密码123-pa55w0rd"},
		{"long-but-under-limit", strings.Repeat("a", 71)},
		{"single-char", "x"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := HashPassword(tc.plain)
			if err != nil {
				t.Fatalf("HashPassword(%q): %v", tc.plain, err)
			}
			if hash == "" {
				t.Fatalf("HashPassword returned empty hash")
			}
			if hash == tc.plain {
				t.Errorf("hash equals plaintext: %q", hash)
			}
			if err := VerifyPassword(hash, tc.plain); err != nil {
				t.Errorf("VerifyPassword with correct plain: %v", err)
			}
		})
	}
}

// TestVerifyPasswordRejectsMismatch is the negative case for the round
// trip: any deviation in the plaintext (different password, extra byte,
// missing byte) must produce a non-nil error. The error itself is
// opaque on purpose; we only assert it is non-nil.
func TestVerifyPasswordRejectsMismatch(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	bad := []string{"hunter3", "hunter", "hunter22", "Hunter2"}
	for _, p := range bad {
		t.Run(p, func(t *testing.T) {
			if err := VerifyPassword(hash, p); err == nil {
				t.Errorf("VerifyPassword(%q) succeeded; want mismatch", p)
			}
		})
	}
}

// TestHashPasswordTooLong surfaces our sentinel mapping: a plaintext
// over bcrypt's 72-byte limit must return ErrPasswordTooLong rather
// than a generic "hash password" wrapper. Callers can then decide
// whether to reject the input rather than treating it as a 500.
func TestHashPasswordTooLong(t *testing.T) {
	plain := strings.Repeat("a", 73)
	_, err := HashPassword(plain)
	if err == nil {
		t.Fatal("HashPassword accepted 73-byte input; want error")
	}
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("HashPassword err = %v, want ErrPasswordTooLong", err)
	}
}

// TestHashIsNotDeterministic guards against the regression where a future
// "optimization" disables the salt. Two hashes of the same plaintext
// must differ.
func TestHashIsNotDeterministic(t *testing.T) {
	h1, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword #1: %v", err)
	}
	h2, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword #2: %v", err)
	}
	if h1 == h2 {
		t.Errorf("two hashes of the same plaintext are identical: %q", h1)
	}
}

// TestBcryptCostOverride exercises the configurable-cost path so operators
// can verify the OPC_BCRYPT_COST knob is wired up end-to-end. We do not
// drive it through the env var (CurrentBcryptCost caches the value on
// first call and the production process resolves it once) because that
// would leak state across the entire test binary; instead, we use the
// SetBcryptCost helper, which is the same code path the env-var
// resolution lands on internally.
func TestBcryptCostOverride(t *testing.T) {
	prev := CurrentBcryptCost()
	t.Cleanup(func() { SetBcryptCost(prev) })

	SetBcryptCost(6) // low enough to keep the test fast
	if got := CurrentBcryptCost(); got != 6 {
		t.Fatalf("CurrentBcryptCost after SetBcryptCost(6) = %d, want 6", got)
	}
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// We do not crack the hash to assert the cost; the contract is
	// "uses the configured cost", and CurrentBcryptCost above is the
	// declarative half of the assertion. We do confirm round-tripping
	// still works at the lower cost so the operator override path is
	// truly end-to-end.
	if err := VerifyPassword(hash, "hunter2"); err != nil {
		t.Errorf("VerifyPassword: %v", err)
	}
}

// TestBcryptCostClamping covers the misconfiguration safety net: a cost
// below MinCost falls back to DefaultCost, and a cost above MaxCost is
// clamped to MaxCost. Without these guards a typo in OPC_BCRYPT_COST
// could either weaken hashes silently or wedge the process behind a
// multi-minute hash.
func TestBcryptCostClamping(t *testing.T) {
	prev := CurrentBcryptCost()
	t.Cleanup(func() { SetBcryptCost(prev) })

	SetBcryptCost(1) // below MinCost
	if got := CurrentBcryptCost(); got < 4 {
		t.Errorf("low-cost clamp: CurrentBcryptCost = %d, want >= bcrypt.MinCost (4)", got)
	}
	SetBcryptCost(99) // above MaxCost
	if got := CurrentBcryptCost(); got > 31 {
		t.Errorf("high-cost clamp: CurrentBcryptCost = %d, want <= bcrypt.MaxCost (31)", got)
	}
}
