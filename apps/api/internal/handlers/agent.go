// Package handlers provides HTTP route handlers for the OPC
// platform. agent.go implements the operator-only admin
// endpoints for managing agent API keys (X-API-Key auth path).
//
// All endpoints require RequireAuth + RequireOperatorRole
// middleware (configured in main.go on the /api/admin group).
// A creator-role session calling any of these endpoints
// receives 403 — that gate lives in the middleware, not in this
// file.
package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// AgentHandler bundles the 4 admin endpoints for managing
// agent API keys. Stateless beyond the *gorm.DB handle, so it
// is safe to construct once in main.go and register on the
// admin group.
type AgentHandler struct {
	db *gorm.DB
}

// NewAgentHandler wires an AgentHandler backed by the given
// *gorm.DB.
func NewAgentHandler(db *gorm.DB) *AgentHandler {
	return &AgentHandler{db: db}
}

// RegisterRoutes attaches the 4 admin endpoints to the router.
// The router group is expected to already be mounted at the
// /api/admin prefix AND to already carry
// RequireAuth + RequireOperatorRole — see cmd/server/main.go.
// The paths below are written RELATIVE to that prefix; an
// absolute "/api/admin/..." form would double the prefix and
// make every request hit NoRoute (404). Sub-Spec C Task 2 had
// this bug; Task 5 caught it via smoke; we keep the relative
// form here from the start.
func (h *AgentHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/agents", h.Create)
	r.GET("/agents", h.List)
	r.POST("/agents/:id/disable", h.Disable)
	r.POST("/agents/:id/regenerate", h.Regenerate)
}

// Create handles POST /api/admin/agents.
//
// Operator-only. Body: {name}. Returns 201 + the new agent
// record + the FULL key, shown once and never again (GitHub
// PAT-style). The CreatedBy field is stamped with the caller's
// user ID so the audit trail records which operator created
// this agent.
func (h *AgentHandler) Create(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required,min=1,max=100"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	key, prefix, err := models.GenerateKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "key generation failed"})
		return
	}
	hash, err := models.HashPassword(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}

	operatorID := UserIDFromContext(c)
	a := models.Agent{
		Name:      req.Name,
		KeyPrefix: prefix,
		HashedKey: hash,
		CreatedBy: operatorID,
		Scope:     "all", // M1: every agent is platform-wide.
	}
	if err := h.db.Create(&a).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed: " + err.Error()})
		return
	}

	// M1: return the full key exactly once. The struct tag
	// json:"-" on Agent.HashedKey means the bcrypt hash is
	// never serialized — only the prefix is exposed in `agent`.
	c.JSON(http.StatusCreated, gin.H{
		"agent": a,
		"key":   key,
	})
}

// List handles GET /api/admin/agents.
//
// Operator-only. Returns 200 + array of agents, newest first.
// The full key is never included in the response; the struct
// tag json:"-" on HashedKey enforces that. The KeyPrefix field
// (12 chars of "opc_agent_XX") is the only key material
// operators see here — enough to identify a key in a console
// or a support ticket without leaking the secret.
func (h *AgentHandler) List(c *gin.Context) {
	var agents []models.Agent
	if err := h.db.Order("created_at DESC").Find(&agents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

// Disable handles POST /api/admin/agents/:id/disable.
//
// Operator-only. Soft-deletes the agent by flipping
// Disabled=true. The row stays in the table for audit; the
// key simply stops authenticating (the call_log path checks
// Disabled before issuing a 200 — verified end-to-end in
// Task 3). Returns 200 + {ok:true, disabled:true}. 404 if the
// target agent does not exist.
func (h *AgentHandler) Disable(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var a models.Agent
	if err := h.db.First(&a, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
		return
	}
	a.Disabled = true
	if err := h.db.Save(&a).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "disabled": true})
}

// Regenerate handles POST /api/admin/agents/:id/regenerate.
//
// Operator-only. Generates a new key, replaces the stored
// bcrypt hash, and clears Disabled + LastUsedAt. The new key
// is returned exactly once — M1 does not surface the old key
// in the response (operators who need to rotate without
// invalidating are out of scope for the first cut). Returns
// 200 + {agent, new_key}. 404 if the target agent does not
// exist.
func (h *AgentHandler) Regenerate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var a models.Agent
	if err := h.db.First(&a, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
		return
	}

	newKey, newPrefix, err := models.GenerateKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "key generation failed"})
		return
	}
	hash, err := models.HashPassword(newKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}

	a.HashedKey = hash
	a.KeyPrefix = newPrefix
	a.Disabled = false
	a.LastUsedAt = nil
	if err := h.db.Save(&a).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"agent":   a,
		"new_key": newKey,
	})
}
