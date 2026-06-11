package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/auth"
	"github.com/opc/api/internal/models"
)

// CredentialHandler exposes the per-tenant API-key management surface.
//
// The handler owns the *gorm.DB and a master key handle. Every
// Encrypt/Decrypt call goes through auth.Encrypt/Decrypt with the
// supplied key so the unit tests can wire a deterministic key
// without ever touching the ENCRYPTION_KEY env var.
//
// Wire shape (all responses):
//
//	POST   /api/credentials        — returns the metadata of the
//	                                  newly created row; EncryptedKey
//	                                  is NEVER echoed back.
//	GET    /api/credentials        — returns the full list of
//	                                  metadata rows (no cipher).
//	PUT    /api/credentials/:id/rotate — same shape as POST.
//	DELETE /api/credentials/:id    — 204 No Content; also writes a
//	                                  call_logs row with skill =
//	                                  "credential_deleted" so the
//	                                  audit trail surfaces deletions
//	                                  alongside the read paths.
type CredentialHandler struct {
	db  *gorm.DB
	key []byte
	log *slog.Logger
	now func() time.Time
}

// NewCredentialHandler wires a handler with the given DB and master
// key (already decoded from ENCRYPTION_KEY). The now func defaults
// to time.Now and is exposed for tests that need a deterministic
// clock. A nil logger falls back to slog.Default(). A short key
// panics — the contract is "32 raw bytes" and a misconfigured
// wiring should fail loud at startup, not 500 on the first request.
func NewCredentialHandler(db *gorm.DB, key []byte, logger *slog.Logger) *CredentialHandler {
	if logger == nil {
		logger = slog.Default()
	}
	if len(key) < auth.EncryptionKeyMinBytes() {
		panic("handlers.NewCredentialHandler: master key is shorter than 32 bytes")
	}
	return &CredentialHandler{
		db:  db,
		key: key,
		log: logger,
		now: time.Now,
	}
}

// RegisterRoutes attaches the four /credentials routes. Mounted
// under the auth-required /api group so the RequireAuth middleware
// runs first; handlers read the user_id off the Gin context.
func (h *CredentialHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/credentials", h.Create)
	r.GET("/credentials", h.List)
	r.PUT("/credentials/:id/rotate", h.Rotate)
	r.DELETE("/credentials/:id", h.Delete)
}

// createRequest is the JSON body for POST /credentials. PlaintextKey
// is the secret the client wants to store; we AES-GCM encrypt it
// before persistence and never echo it back.
type createRequest struct {
	Provider     string `json:"provider"`
	Name         string `json:"name"`
	PlaintextKey string `json:"plaintext_key"`
	Scope        string `json:"scope"`
}

// rotateRequest is the JSON body for PUT /credentials/:id/rotate.
// We deliberately do NOT support name/scope changes on rotate —
// that's an "update" verb in API design parlance, and the
// operational meaning of "rotate" should stay narrow: swap the
// secret, keep everything else.
type rotateRequest struct {
	PlaintextKey string `json:"plaintext_key"`
}

// Create — POST /credentials
//
// Body: {provider, name, plaintext_key, scope?}
// 201:  {credential metadata, NO encrypted_key}
// 400:  invalid body, missing field, or provider out of set
// 500:  encrypt / insert failure
func (h *CredentialHandler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Warn("invalid credential body", "class", bindErrorClass(err), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.PlaintextKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plaintext_key is required"})
		return
	}
	cred := models.Credential{
		UserID:   UserIDFromContext(c),
		Provider: req.Provider,
		Name:     req.Name,
		Scope:    req.Scope,
	}
	if err := cred.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cipher, err := auth.Encrypt([]byte(req.PlaintextKey), h.key)
	if err != nil {
		h.log.Error("encrypt credential failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store credential"})
		return
	}
	cred.EncryptedKey = cipher
	if err := h.db.WithContext(c.Request.Context()).Create(&cred).Error; err != nil {
		h.log.Error("insert credential failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store credential"})
		return
	}
	c.JSON(http.StatusCreated, toCredentialResponse(cred))
}

// List — GET /credentials
//
// Returns the metadata of every credential owned by the caller,
// newest-first. The encrypted_key column is intentionally absent
// from the response struct so even a future "include cipher" toggle
// can't accidentally leak it without a code change here.
func (h *CredentialHandler) List(c *gin.Context) {
	userID := UserIDFromContext(c)
	if err := requireUserID(userID); err != nil {
		h.log.Error("list credentials: missing user id", "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list credentials"})
		return
	}
	var rows []models.Credential
	if err := h.db.WithContext(c.Request.Context()).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		h.log.Error("list credentials failed", "err", err.Error(), "user_id", userID, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list credentials"})
		return
	}
	out := make([]credentialResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCredentialResponse(r))
	}
	c.JSON(http.StatusOK, gin.H{"items": out, "total": len(out)})
}

// Rotate — PUT /credentials/:id/rotate
//
// Body: {plaintext_key}
// 200:  {credential metadata, NO encrypted_key}
// 400:  invalid body or empty plaintext
// 404:  no row with that id under this user
// 500:  encrypt / update failure
//
// The handler does an ownership-checked SELECT, re-encrypts, and
// writes a new UpdatedAt in one Save call. We do NOT use the
// pattern of "encrypt then UPDATE WHERE id=user" because that
// silently mutates a row owned by another tenant if the ID is
// guessed — the SELECT-then-SAVE path makes the tenant leak
// loud (a 404).
func (h *CredentialHandler) Rotate(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	var req rotateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.log.Warn("invalid rotate body", "class", bindErrorClass(err), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.PlaintextKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plaintext_key is required"})
		return
	}
	userID := UserIDFromContext(c)
	if err := requireUserID(userID); err != nil {
		h.log.Error("rotate credential: missing user id", "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate credential"})
		return
	}
	var cred models.Credential
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND user_id = ?", id, userID).
		First(&cred).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "credential not found"})
			return
		}
		h.log.Error("rotate: lookup failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate credential"})
		return
	}
	cipher, err := auth.Encrypt([]byte(req.PlaintextKey), h.key)
	if err != nil {
		h.log.Error("rotate: encrypt failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate credential"})
		return
	}
	cred.EncryptedKey = cipher
	cred.UpdatedAt = h.now().UTC()
	if err := h.db.WithContext(c.Request.Context()).Save(&cred).Error; err != nil {
		h.log.Error("rotate: save failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate credential"})
		return
	}
	c.JSON(http.StatusOK, toCredentialResponse(cred))
}

// Delete — DELETE /credentials/:id
//
// 204:  on success
// 404:  no row with that id under this user
// 500:  delete failure
//
// Side effect: writes a call_logs row with skill =
// "credential_deleted" so the audit trail surfaces deletions
// alongside read/write traffic. The credential_id is null on the
// audit row (the row no longer exists), but the user_id and
// provider stay attached for grouping.
func (h *CredentialHandler) Delete(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	userID := UserIDFromContext(c)
	if err := requireUserID(userID); err != nil {
		h.log.Error("delete credential: missing user id", "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete credential"})
		return
	}
	var cred models.Credential
	if err := h.db.WithContext(c.Request.Context()).
		Where("id = ? AND user_id = ?", id, userID).
		First(&cred).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "credential not found"})
			return
		}
		h.log.Error("delete: lookup failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete credential"})
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Delete(&cred).Error; err != nil {
		h.log.Error("delete: db delete failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete credential"})
		return
	}
	// Audit row. Best-effort: a failure here is logged but does
	// not undo the credential delete (the row is gone, and the
	// operator's primary concern is the credential, not the
	// log). The call_log table is owned by the observability
	// surface; we keep the write site in the handler so the
	// delete-side audit does not depend on a middleware being on
	// the call stack.
	audit := models.CallLog{
		UserID:    userID,
		Skill:     "credential_deleted",
		Provider:  cred.Provider,
		Status:    models.CallStatusOK,
		CreatedAt: h.now().UTC(),
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&audit).Error; err != nil {
		h.log.Warn("credential delete audit row failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
	}
	c.Status(http.StatusNoContent)
}

// credentialResponse is the public wire shape. EncryptedKey is
// explicitly absent — the struct fields map directly to JSON keys
// and the column simply isn't here, so a future maintainer cannot
// accidentally add it back without a deliberate change.
type credentialResponse struct {
	ID         uint       `json:"id"`
	UserID     uint       `json:"user_id"`
	Provider   string     `json:"provider"`
	Name       string     `json:"name"`
	Scope      string     `json:"scope"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func toCredentialResponse(c models.Credential) credentialResponse {
	return credentialResponse{
		ID:         c.ID,
		UserID:     c.UserID,
		Provider:   c.Provider,
		Name:       c.Name,
		Scope:      c.Scope,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
		LastUsedAt: c.LastUsedAt,
	}
}
