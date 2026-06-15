// Package handlers provides HTTP route handlers for the OPC
// platform. creator.go implements the operator-only admin
// endpoints for managing external beta creators (Sub-Spec C M1
// Task 2).
//
// All endpoints require RequireAuth + RequireOperatorRole
// middleware (configured in main.go). A creator-role session
// calling any of these endpoints receives 403 — that gate lives
// in the middleware, not in this file.
package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/opc/api/internal/models"
)

// CreatorHandler bundles the 4 admin endpoints for managing
// external creators. Stateless beyond the *gorm.DB handle, so it
// is safe to construct once in main.go and register on the
// admin group.
type CreatorHandler struct {
	db *gorm.DB
}

// NewCreatorHandler wires a CreatorHandler backed by the given
// *gorm.DB.
func NewCreatorHandler(db *gorm.DB) *CreatorHandler {
	return &CreatorHandler{db: db}
}

// RegisterRoutes attaches the 4 admin endpoints to the router.
// The router group is expected to already carry
// RequireAuth + RequireOperatorRole — see cmd/server/main.go.
func (h *CreatorHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/api/admin/creators", h.CreateCreator)
	r.GET("/api/admin/creators", h.ListCreators)
	r.POST("/api/admin/creators/:id/reset-password", h.ResetPassword)
	r.POST("/api/admin/creators/:id/disable", h.DisableCreator)
}

// CreateCreator handles POST /api/admin/creators.
//
// Operator-only. Body: {email, password}. Returns 201 + the new
// user. The CreatedBy field is stamped with the caller's user ID
// so the audit trail records which operator created this creator.
func (h *CreatorHandler) CreateCreator(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}

	operatorID := UserIDFromContext(c)
	u := models.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		Role:         "creator",
		CreatedBy:    operatorID,
	}
	if err := h.db.Create(&u).Error; err != nil {
		// TranslateError is enabled at the DB layer, so we can rely
		// on gorm.ErrDuplicatedKey rather than matching on the
		// SQLite-specific "UNIQUE constraint failed" English text.
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "UNIQUE constraint") {
			c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": u})
}

// ListCreators handles GET /api/admin/creators.
//
// Operator-only. Returns 200 + array of creator-role users,
// newest first. Operators are NOT included in the listing —
// this surface is for managing external creators, not the
// internal operator roster.
func (h *CreatorHandler) ListCreators(c *gin.Context) {
	var creators []models.User
	if err := h.db.Where("role = ?", "creator").Order("created_at DESC").Find(&creators).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"creators": creators})
}

// ResetPassword handles POST /api/admin/creators/:id/reset-password.
//
// Operator-only. Body: {new_password}. Returns 200 on success.
// 404 if the target user does not exist; 400 if the target is
// not a creator (the endpoint is creator-scoped — operators
// manage their own passwords via /api/auth/me).
func (h *CreatorHandler) ResetPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	var u models.User
	if err := h.db.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "creator not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
		return
	}
	if u.Role != "creator" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a creator"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}
	u.PasswordHash = string(hash)
	if err := h.db.Save(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DisableCreator handles POST /api/admin/creators/:id/disable.
//
// Operator-only. Soft-deletes the creator by flipping Disabled=true.
// Returns 200 + {ok:true, disabled:true}. 404 if the target does
// not exist; 400 if the target is not a creator.
func (h *CreatorHandler) DisableCreator(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var u models.User
	if err := h.db.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "creator not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
		return
	}
	if u.Role != "creator" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a creator"})
		return
	}

	u.Disabled = true
	if err := h.db.Save(&u).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "disabled": true})
}