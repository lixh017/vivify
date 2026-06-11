package models

import (
	"errors"
	"strings"
	"time"
)

// CredentialProvider enumerates the upstream APIs a tenant may store a
// secret for. Phase 3 (B 端控制台) only needs a closed set; new
// providers should be added here AND to ProviderIsAllowed so the API
// boundary rejects anything else.
//
// Douyin is reserved for a later phase — keeping the constant now
// means we don't have to migrate the table or rewire handlers when
// the operator onboards 抖音开放平台. Until then POST /credentials
// with provider=douyin is accepted and stored, but no caller reads
// the row yet.
const (
	ProviderVolcengine = "volcengine"
	ProviderAnthropic  = "anthropic"
	ProviderDouyin     = "douyin"
)

// ProviderIsAllowed reports whether provider is one of the values
// Credential accepts. The check is case-insensitive so "Volcengine"
// and "volcengine" both pass; we normalize the stored value to
// lowercase in Validate.
func ProviderIsAllowed(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case ProviderVolcengine, ProviderAnthropic, ProviderDouyin:
		return true
	default:
		return false
	}
}

// Credential is one tenant-scoped upstream secret (volcengine /
// anthropic / douyin). The plaintext is never persisted: we AES-GCM
// encrypt it in the handler and store the ciphertext in
// EncryptedKey. The wire shape (and any GORM JSON marshalling) MUST
// NOT include the field, which is why the struct tag is `json:"-"`.
//
// UserID is the multi-tenant key — the Phase 2 RequireAuth middleware
// stamps the caller, and every store method WHERE's on it. There is
// no admin override path; a future "operator impersonation" feature
// would still have to write rows with a real user_id.
//
// Scope is reserved for a future skill-scope filter ("image-only" /
// "tts-only" / "all"). The default value is "all" so existing
// rows produced before the scope concept land still match every
// skill. Empty/whitespace input is rejected by Validate; the handler
// is responsible for defaulting to "all" if the client omits the
// field.
type Credential struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	UserID       uint       `gorm:"index" json:"user_id"`
	Provider     string     `gorm:"index" json:"provider"`
	Name         string     `json:"name"`
	EncryptedKey []byte     `json:"-"` // never expose
	Scope        string     `json:"scope"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}

// TableName pins the underlying SQLite table name so the model is
// unambiguous in logs, migrations, and the migrate tests' table list.
func (Credential) TableName() string { return "credentials" }

// ErrCredentialInvalid is the sentinel for validation failures
// surfaced by Validate. Callers (the HTTP handler) map it to 400
// without echoing the inner message verbatim.
var ErrCredentialInvalid = errors.New("credential: invalid")

// Validate normalizes the provider/scope and rejects empty /
// out-of-set values. Called by the handler before persisting so
// bad rows never reach the DB.
func (c *Credential) Validate() error {
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	c.Scope = strings.ToLower(strings.TrimSpace(c.Scope))
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return wrapInvalid("name is required")
	}
	if !ProviderIsAllowed(c.Provider) {
		return wrapInvalid("provider is not an allowed value")
	}
	if c.Scope == "" {
		c.Scope = "all"
	}
	return nil
}

func wrapInvalid(msg string) error {
	// wrapInvalid joins ErrCredentialInvalid with the human-readable
	// detail. We intentionally keep this in models (not in a separate
	// validation package) so the rule is co-located with the model
	// shape it constrains.
	return errors.Join(ErrCredentialInvalid, errors.New(msg))
}
