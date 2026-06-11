package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
)

// testKey is a deterministic 32-byte key used by every crypto
// round-trip test. We do NOT read it from the env so the test
// outcome is reproducible regardless of how the developer started
// the suite. The bytes are random-looking; what matters is that
// the same value is used for both Encrypt and Decrypt on each
// test invocation.
var testKey = bytes.Repeat([]byte{0x42}, 32)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		plaintext []byte
	}{
		{"empty", []byte("")},
		{"short", []byte("k-123")},
		{"anthropic-style", []byte("sk-ant-api03-abcdef1234567890")},
		{"volcengine-ak-sk", []byte("AKLT" + strings.Repeat("A", 40) + "\nSK" + strings.Repeat("B", 60))},
		{"binary", []byte{0x00, 0xff, 0x10, 0x20, 0x30, 0x40}},
		{"unicode", []byte("凭据-密钥-🔐")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blob, err := Encrypt(tc.plaintext, testKey)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			if len(blob) < nonceSize {
				t.Fatalf("ciphertext is shorter than nonce: %d", len(blob))
			}
			got, err := Decrypt(blob, testKey)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(got, tc.plaintext) {
				t.Fatalf("round-trip mismatch: got %q, want %q", got, tc.plaintext)
			}
		})
	}
}

// TestEncryptNonceUniqueness exercises the GCM property that
// repeated calls produce different nonces. With 12 random bytes
// per call the probability of collision is ~2^-48, so even a
// small N is enough to trip a regression that hardcoded the
// nonce (or skipped it). We do not assert non-collision — a
// single dup would flake the test; the property we DO assert is
// that the leading nonce bytes differ across calls (sufficient
// for catching a hardcoded-nonce regression).
func TestEncryptNonceUniqueness(t *testing.T) {
	const n = 8
	first := make([]byte, nonceSize)
	for i := 0; i < n; i++ {
		blob, err := Encrypt([]byte("same"), testKey)
		if err != nil {
			t.Fatalf("Encrypt #%d: %v", i, err)
		}
		if i == 0 {
			copy(first, blob[:nonceSize])
			continue
		}
		if bytes.Equal(blob[:nonceSize], first) {
			t.Fatalf("nonce repeated on call %d", i)
		}
	}
}

// TestEncryptShortKey asserts the validation path. A key shorter
// than 32 bytes is a programmer/operator error; we surface a
// stable sentinel (ErrEncryptionKeyShort) so the caller can map
// the failure to a clean error message.
func TestEncryptShortKey(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		[]byte("too-short"),
		bytes.Repeat([]byte{0x01}, 16), // AES-128 length, not allowed here
		bytes.Repeat([]byte{0x01}, 24), // AES-192 length, not allowed here
		bytes.Repeat([]byte{0x01}, 31), // one byte short of 32
	}
	for i, k := range cases {
		_, err := Encrypt([]byte("hello"), k)
		if !errors.Is(err, ErrEncryptionKeyShort) {
			t.Fatalf("case %d: expected ErrEncryptionKeyShort, got %v", i, err)
		}
	}
}

// TestDecryptShortKey mirrors TestEncryptShortKey. Decrypt must
// fail on the same key-length contract; we don't want a code
// path that loads the key but never validates it.
func TestDecryptShortKey(t *testing.T) {
	_, err := Decrypt(make([]byte, nonceSize+16), []byte("short"))
	if !errors.Is(err, ErrEncryptionKeyShort) {
		t.Fatalf("expected ErrEncryptionKeyShort, got %v", err)
	}
}

// TestDecryptRejectsTamperedCiphertext asserts GCM's
// authenticated-decryption property: any single-byte flip in the
// ciphertext must be rejected. We do not assert the specific
// error type — only that Decrypt returns non-nil.
func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	blob, err := Encrypt([]byte("the magic words are squeamish ossifrage"), testKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Flip a byte in the ciphertext body (past the nonce).
	blob[nonceSize+3] ^= 0x80
	if _, err := Decrypt(blob, testKey); err == nil {
		t.Fatalf("Decrypt accepted tampered ciphertext")
	}
}

// TestDecryptRejectsTruncatedInput covers the "shorter than
// nonce" guard. The function must not panic; it must return
// ErrCiphertextTooShort.
func TestDecryptRejectsTruncatedInput(t *testing.T) {
	for _, n := range []int{0, 1, nonceSize - 1} {
		_, err := Decrypt(make([]byte, n), testKey)
		if !errors.Is(err, ErrCiphertextTooShort) {
			t.Fatalf("len=%d: expected ErrCiphertextTooShort, got %v", n, err)
		}
	}
}

// TestDecryptRejectsWrongKey covers the cross-key path: a blob
// encrypted under testKey must not decrypt under a different
// 32-byte key. We use a different deterministic 32-byte key
// rather than a random one so the test is reproducible.
func TestDecryptRejectsWrongKey(t *testing.T) {
	blob, err := Encrypt([]byte("secret"), testKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	otherKey := bytes.Repeat([]byte{0x33}, 32)
	if _, err := Decrypt(blob, otherKey); err == nil {
		t.Fatalf("Decrypt accepted ciphertext from a different key")
	}
}

// TestLoadKeyMissing covers the env-var-not-set path. The cached
// pointer is reset before and after the test so the test is
// order-independent.
func TestLoadKeyMissing(t *testing.T) {
	// Save and clear.
	prev := os.Getenv(EncryptionKeyEnv)
	t.Cleanup(func() { _ = os.Setenv(EncryptionKeyEnv, prev) })
	_ = os.Unsetenv(EncryptionKeyEnv)
	SetEncryptionKey(nil) // force re-read on next LoadKey

	if _, err := LoadKey(); !errors.Is(err, ErrEncryptionKeyMissing) {
		t.Fatalf("expected ErrEncryptionKeyMissing, got %v", err)
	}
}

// TestLoadKeyShort covers the "decoded length < 32" path. We set
// the env to a 1-byte value (after base64 decode) and assert
// the error. SetEncryptionKey is reset after the test so other
// tests in the package see a clean cache.
func TestLoadKeyShort(t *testing.T) {
	prev := os.Getenv(EncryptionKeyEnv)
	t.Cleanup(func() { _ = os.Setenv(EncryptionKeyEnv, prev) })
	// 16 base64 chars encode to 12 bytes — comfortably under 32.
	_ = os.Setenv(EncryptionKeyEnv, base64.StdEncoding.EncodeToString([]byte("0123456789ab")))
	SetEncryptionKey(nil)
	if _, err := LoadKey(); !errors.Is(err, ErrEncryptionKeyShort) {
		t.Fatalf("expected ErrEncryptionKeyShort, got %v", err)
	}
}

// TestLoadKeyBase64Variants confirms the four flavours (std /
// url / raw-std / raw-url) all decode. We don't need to test
// every case of every flavour — a single example of each is
// enough to catch a regression that drops one branch.
func TestLoadKeyBase64Variants(t *testing.T) {
	cases := map[string]string{
		"std":    base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"url":    base64.URLEncoding.EncodeToString(make([]byte, 32)),
		"rawStd": base64.RawStdEncoding.EncodeToString(make([]byte, 32)),
		"rawURL": base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
	}
	for name, enc := range cases {
		t.Run(name, func(t *testing.T) {
			prev := os.Getenv(EncryptionKeyEnv)
			t.Cleanup(func() { _ = os.Setenv(EncryptionKeyEnv, prev) })
			_ = os.Setenv(EncryptionKeyEnv, enc)
			SetEncryptionKey(nil)
			k, err := LoadKey()
			if err != nil {
				t.Fatalf("LoadKey: %v", err)
			}
			if len(k) != 32 {
				t.Fatalf("len(k) = %d, want 32", len(k))
			}
		})
	}
}

// TestSetEncryptionKey pins the constructor contract: nil/short
// is rejected; 32+ bytes is accepted.
func TestSetEncryptionKey(t *testing.T) {
	if err := SetEncryptionKey(nil); err != nil {
		t.Fatalf("SetEncryptionKey(nil) returned %v, want nil", err)
	}
	if err := SetEncryptionKey([]byte("short")); !errors.Is(err, ErrEncryptionKeyShort) {
		t.Fatalf("SetEncryptionKey(short) returned %v, want ErrEncryptionKeyShort", err)
	}
	if err := SetEncryptionKey(bytes.Repeat([]byte{0x01}, 32)); err != nil {
		t.Fatalf("SetEncryptionKey(32) returned %v, want nil", err)
	}
	if err := SetEncryptionKey(bytes.Repeat([]byte{0x01}, 64)); err != nil {
		t.Fatalf("SetEncryptionKey(64) returned %v, want nil", err)
	}
}

// TestMustLoadKey covers both the happy path and the panic path
// without leaking goroutines or stack frames across tests.
func TestMustLoadKey(t *testing.T) {
	prev := os.Getenv(EncryptionKeyEnv)
	t.Cleanup(func() { _ = os.Setenv(EncryptionKeyEnv, prev) })
	_ = os.Setenv(EncryptionKeyEnv, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	SetEncryptionKey(nil)
	if k := MustLoadKey(); len(k) != 32 {
		t.Fatalf("MustLoadKey returned %d bytes, want 32", len(k))
	}

	_ = os.Unsetenv(EncryptionKeyEnv)
	SetEncryptionKey(nil)
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("MustLoadKey should panic on missing env")
		}
	}()
	_ = MustLoadKey()
}

// TestEncryptRandomnessSpotCheck is a smoke test that catches a
// future regression where Encrypt uses a deterministic IV (e.g.
// a counter that starts at 0 across processes). The probability
// of two random 12-byte blocks colliding is 2^-48; we sample
// 16 ciphertexts and check no two leading 12-byte prefixes are
// equal.
func TestEncryptRandomnessSpotCheck(t *testing.T) {
	const n = 16
	seen := make(map[[nonceSize]byte]bool, n)
	for i := 0; i < n; i++ {
		blob, err := Encrypt([]byte("x"), testKey)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		var prefix [nonceSize]byte
		copy(prefix[:], blob[:nonceSize])
		if seen[prefix] {
			t.Fatalf("nonce collision on call %d", i)
		}
		seen[prefix] = true
	}
	// One more sanity: a fresh random 32-byte key produces a
	// ciphertext that the test key cannot decrypt.
	fresh := make([]byte, 32)
	if _, err := rand.Read(fresh); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	blob, err := Encrypt([]byte("payload"), fresh)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := Decrypt(blob, testKey); err == nil {
		t.Fatalf("Decrypt accepted ciphertext from a fresh random key")
	}
}
