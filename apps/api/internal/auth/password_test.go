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
