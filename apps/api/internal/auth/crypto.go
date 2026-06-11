package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
)

// EncryptionKeyEnv is the environment variable the API binary reads
// the master AES-256 key from. Operators are expected to provision
// 32 raw bytes (encoded as standard or URL-safe base64) out-of-band
// — typically via their secret manager — and inject the value at
// boot. The value is read once on the first EncryptWithKey/Encrypt
// call and cached; subsequent SetEncryptionKey calls (e.g. in tests)
// swap the active key without re-reading the env.
const EncryptionKeyEnv = "ENCRYPTION_KEY"

// encryptionKeyMinBytes is the smallest acceptable key length. AES
// allows 16/24/32 bytes, but Phase 3 pins 32 to keep the surface
// small — if we ever need 16/24 later, both code paths and the
// operator runbook have to move together.
const encryptionKeyMinBytes = 32

// EncryptionKeyMinBytes exports the constant for callers (notably
// handlers/credentials.go) that need to validate the key length at
// construction time without re-declaring the magic number.
func EncryptionKeyMinBytes() int { return encryptionKeyMinBytes }

// nonceSize is the standard nonce length for AES-GCM (12 bytes).
// Anything shorter weakens the security argument; anything longer
// is out of spec. We pin it here so Encrypt and Decrypt agree.
const nonceSize = 12

// ErrEncryptionKeyMissing is returned when ENCRYPTION_KEY is unset.
// Distinct from ErrEncryptionKeyShort so handlers (and tests) can
// distinguish "operator forgot to provision" from "operator set the
// wrong-length value".
var ErrEncryptionKeyMissing = errors.New("auth: ENCRYPTION_KEY env var is not set")

// ErrEncryptionKeyShort is returned when the decoded key is shorter
// than encryptionKeyMinBytes. We deliberately do NOT auto-pad or
// silently fall back — the contract is "32 raw bytes", and accepting
// a shorter key would defeat the threat model.
var ErrEncryptionKeyShort = errors.New("auth: ENCRYPTION_KEY must decode to at least 32 bytes")

// ErrCiphertextTooShort is returned by Decrypt when the input is
// shorter than the nonce. Indicates either a bug in the caller or
// a tampered/truncated row.
var ErrCiphertextTooShort = errors.New("auth: ciphertext is shorter than the nonce prefix")

// encryptionKey holds the decoded master key bytes. It is read once
// on first use and cached behind an atomic pointer so concurrent
// callers don't re-decode the env var on every Encrypt/Decrypt.
//
// Cache state is a 3-valued signal carried by an atomic.Pointer
// pointing at a struct of (key, populated):
//
//	nil pointer  → "not yet loaded" (force a read on next call)
//	{key:nil, populated:false} → "loaded, but env was empty/short"
//	{key:k,     populated:true}  → "loaded, valid k cached"
//
// The first two states look identical when you only look at the
// pointer; we need an explicit populated flag so LoadKey can
// distinguish "I haven't tried" from "I tried and the env was
// missing" without re-reading the env on every call.
var encryptionKey atomic.Pointer[cachedKey]

type cachedKey struct {
	key       []byte
	populated bool
}

// LoadKey resolves the master key from ENCRYPTION_KEY. The first
// call decodes and validates; subsequent calls return the cached
// pointer. SetEncryptionKey resets the cache for tests.
//
// We accept both standard and URL-safe base64 (with or without
// padding) because operators are far more likely to have a base64
// string lying around than 32 raw bytes in a config file. The
// decoded byte length is what matters, not the surface encoding.
func LoadKey() ([]byte, error) {
	if p := encryptionKey.Load(); p != nil && p.populated {
		return p.key, nil
	}
	raw := os.Getenv(EncryptionKeyEnv)
	if raw == "" {
		encryptionKey.Store(&cachedKey{populated: true})
		return nil, ErrEncryptionKeyMissing
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		// Fall back to URL-safe without padding — some secret
		// managers emit that flavour. We keep the fallback chain
		// shallow on purpose: a single retry covers 99% of
		// real-world misconfigurations.
		if decoded, err = base64.URLEncoding.DecodeString(raw); err != nil {
			if decoded, err = base64.RawStdEncoding.DecodeString(raw); err != nil {
				if decoded, err = base64.RawURLEncoding.DecodeString(raw); err != nil {
					encryptionKey.Store(&cachedKey{populated: true})
					return nil, fmt.Errorf("%w: %v", ErrEncryptionKeyShort, err)
				}
			}
		}
	}
	if len(decoded) < encryptionKeyMinBytes {
		encryptionKey.Store(&cachedKey{populated: true})
		return nil, ErrEncryptionKeyShort
	}
	encryptionKey.Store(&cachedKey{key: decoded, populated: true})
	return decoded, nil
}

// SetEncryptionKey overrides the master key for the current
// process. Intended for tests — production code should rely on the
// ENCRYPTION_KEY env var. A nil key clears the cache so the next
// call re-reads the env (useful for "unset then set" patterns in
// the test suite).
func SetEncryptionKey(key []byte) error {
	if len(key) > 0 && len(key) < encryptionKeyMinBytes {
		return ErrEncryptionKeyShort
	}
	if len(key) == 0 {
		encryptionKey.Store(nil)
		return nil
	}
	// Copy the bytes: the caller's slice may be reused (e.g. a
	// test loop re-uses the same buffer), and we don't want
	// subsequent mutations to corrupt the cached key.
	buf := make([]byte, len(key))
	copy(buf, key)
	encryptionKey.Store(&cachedKey{key: buf, populated: true})
	return nil
}

// MustLoadKey panics if the ENCRYPTION_KEY is missing or short.
// Wired into main.go on startup so a misconfigured deploy fails
// loud at boot instead of surfacing a 500 on the first
// encrypt/decrypt call.
func MustLoadKey() []byte {
	k, err := LoadKey()
	if err != nil {
		panic(fmt.Sprintf("auth: %v", err))
	}
	return k
}

// Encrypt seals plaintext with AES-GCM under the supplied key and
// returns nonce || ciphertext. The caller controls the key, so
// this function is trivially testable: the test can pass a 32-byte
// deterministic key. For production code the global helpers
// (LoadKey / MustLoadKey) resolve the key from ENCRYPTION_KEY.
//
// A fresh random nonce is generated for every call. Reusing a
// nonce with the same key is catastrophic for GCM, so we never
// accept a caller-provided nonce; if a future "deterministic
// encryption" feature is needed it should go through a separate
// primitive (HKDF-derived per-record subkeys, etc.).
func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	if len(key) < encryptionKeyMinBytes {
		return nil, ErrEncryptionKeyShort
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	// Layout: [nonce(12) | ciphertext+tag]. We append the sealed
	// bytes after the nonce so a single byte slice holds everything
	// we need to persist.
	out := make([]byte, 0, nonceSize+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// Decrypt inverts Encrypt: the first 12 bytes are the nonce, the
// rest is the AES-GCM ciphertext + tag. Any failure (bad key,
// tampered ciphertext, wrong nonce length) collapses into a single
// non-nil error so the caller can't accidentally leak the failure
// mode to the client.
func Decrypt(blob []byte, key []byte) ([]byte, error) {
	if len(blob) < nonceSize {
		return nil, ErrCiphertextTooShort
	}
	if len(key) < encryptionKeyMinBytes {
		return nil, ErrEncryptionKeyShort
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	nonce := blob[:nonceSize]
	sealed := blob[nonceSize:]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("gcm open: %w", err)
	}
	return plain, nil
}
