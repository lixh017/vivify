package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/opc/api/internal/models"
)

// KnowledgeDocStore is the persistence contract the KnowledgeDocHandler
// needs. It is intentionally narrow so tests can supply lightweight fakes
// and so the handler is not coupled to GORM. The interface is defined
// where it is consumed, per the "accept interfaces, return structs" Go
// idiom.
type KnowledgeDocStore interface {
	Create(ctx context.Context, k *models.KnowledgeDoc) error
	List(ctx context.Context, filter KnowledgeDocFilter, page PageRequest) ([]models.KnowledgeDoc, int64, error)
	Get(ctx context.Context, id uint, userID uint) (*models.KnowledgeDoc, error)
	Update(ctx context.Context, id uint, patch map[string]any, userID uint) (*models.KnowledgeDoc, error)
	Delete(ctx context.Context, id uint, userID uint) (int64, error)
}

// KnowledgeDocFilter narrows List results. Zero value means "no filter".
// DocType is the primary task-mandated filter; UserID is the Phase 2
// owner scope (RequireAuth sets it before List is called).
type KnowledgeDocFilter struct {
	UserID  uint
	DocType string
}

// KnowledgeDocHandler exposes the knowledge doc REST surface. The store
// field is an interface so we can swap in fakes in unit tests; production
// wiring passes the *gormKnowledgeDocStore adapter defined in this file.
type KnowledgeDocHandler struct {
	store  KnowledgeDocStore
	logger *slog.Logger
}

// NewKnowledgeDocHandler wires a KnowledgeDocHandler backed by the given
// *gorm.DB. This is the primary constructor used by main.go and by tests
// that want a real (in-memory) database.
func NewKnowledgeDocHandler(db *gorm.DB) *KnowledgeDocHandler {
	return NewKnowledgeDocHandlerWithStore(newGormKnowledgeDocStore(db), nil)
}

// NewKnowledgeDocHandlerWithStore wires a KnowledgeDocHandler against the
// given store and logger. A nil logger falls back to slog.Default(). Used
// by tests that want to inject a fake store.
func NewKnowledgeDocHandlerWithStore(store KnowledgeDocStore, logger *slog.Logger) *KnowledgeDocHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &KnowledgeDocHandler{store: store, logger: logger}
}

// RegisterRoutes attaches the CRUD routes for /knowledge to the given
// router. The handler owns its own URL space so main.go and tests do not
// duplicate wiring.
func (h *KnowledgeDocHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/knowledge", h.Create)
	r.GET("/knowledge", h.List)
	r.GET("/knowledge/:id", h.Get)
	r.PUT("/knowledge/:id", h.Update)
	r.DELETE("/knowledge/:id", h.Delete)
}

// Create — POST /knowledge
func (h *KnowledgeDocHandler) Create(c *gin.Context) {
	var kd models.KnowledgeDoc
	if err := c.ShouldBindJSON(&kd); err != nil {
		h.logger.Warn("invalid create body", "err", err.Error(), "request_id", c.GetString("request_id"))
		msg := "invalid request body"
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		if errors.As(err, &syntaxErr) || errors.As(err, &unmarshalErr) || errors.Is(err, io.EOF) {
			// keep generic
		} else {
			msg = err.Error()
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	kd.UserID = UserIDFromContext(c)
	if err := h.store.Create(c.Request.Context(), &kd); err != nil {
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("create knowledge doc failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create knowledge doc"})
		return
	}
	c.JSON(http.StatusCreated, kd)
}

// List — GET /knowledge?doc_type=&limit=&offset=
//
// doc_type is the primary task-mandated filter. The current schema allows
// any non-empty string, so we do not validate against a closed enum.
func (h *KnowledgeDocHandler) List(c *gin.Context) {
	filter := KnowledgeDocFilter{
		UserID:  UserIDFromContext(c),
		DocType: strings.TrimSpace(c.Query("doc_type")),
	}

	page := PageRequest{Limit: defaultListPageSize, Offset: 0}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be a positive integer"})
			return
		}
		if n > maxListPageSize {
			n = maxListPageSize
		}
		page.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be a non-negative integer"})
			return
		}
		page.Offset = n
	}

	items, total, err := h.store.List(c.Request.Context(), filter, page)
	if err != nil {
		h.logger.Error("list knowledge docs failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list knowledge docs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  page.Limit,
		"offset": page.Offset,
	})
}

// Get — GET /knowledge/:id
func (h *KnowledgeDocHandler) Get(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	kd, err := h.store.Get(c.Request.Context(), id, UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "knowledge doc not found"})
			return
		}
		h.logger.Error("get knowledge doc failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get knowledge doc"})
		return
	}
	c.JSON(http.StatusOK, kd)
}

// Update — PUT /knowledge/:id. Accepts a partial-update map so omitted
// fields do not silently clobber existing values (the classic
// ShouldBindJSON-into-existing-struct pitfall). The store performs the
// lookup + save in a transaction to avoid TOCTOU races.
func (h *KnowledgeDocHandler) Update(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	var patch map[string]any
	if err := c.ShouldBindJSON(&patch); err != nil {
		h.logger.Warn("invalid update body", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	// Strip the primary key; the URL owns the identity.
	delete(patch, "id")
	delete(patch, "created_at")
	delete(patch, "updated_at")

	kd, err := h.store.Update(c.Request.Context(), id, patch, UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "knowledge doc not found"})
			return
		}
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("update knowledge doc failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update knowledge doc"})
		return
	}
	c.JSON(http.StatusOK, kd)
}

// Delete — DELETE /knowledge/:id. Returns 200 with {deleted: id} on
// success, 404 if the row did not exist.
func (h *KnowledgeDocHandler) Delete(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	rows, err := h.store.Delete(c.Request.Context(), id, UserIDFromContext(c))
	if err != nil {
		h.logger.Error("delete knowledge doc failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete knowledge doc"})
		return
	}
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "knowledge doc not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// gormKnowledgeDocStore is the production KnowledgeDocStore backed by
// GORM. It binds every call to a bounded context so a slow query cannot
// outlive the request.
type gormKnowledgeDocStore struct {
	db *gorm.DB
}

func newGormKnowledgeDocStore(db *gorm.DB) *gormKnowledgeDocStore {
	return &gormKnowledgeDocStore{db: db}
}

func (s *gormKnowledgeDocStore) Create(ctx context.Context, kd *models.KnowledgeDoc) error {
	return s.db.WithContext(withTimeout(ctx)).Create(kd).Error
}

func (s *gormKnowledgeDocStore) List(ctx context.Context, f KnowledgeDocFilter, p PageRequest) ([]models.KnowledgeDoc, int64, error) {
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.KnowledgeDoc{}).Order(
		clause.OrderByColumn{Column: clause.Column{Name: "created_at"}, Desc: true},
	)
	if f.UserID != 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.DocType != "" {
		q = q.Where("doc_type = ?", f.DocType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.KnowledgeDoc
	if err := q.Limit(p.Limit).Offset(p.Offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *gormKnowledgeDocStore) Get(ctx context.Context, id uint, userID uint) (*models.KnowledgeDoc, error) {
	var kd models.KnowledgeDoc
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.KnowledgeDoc{})
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&kd, id).Error; err != nil {
		return nil, err
	}
	return &kd, nil
}

// Update wraps the read+write in a transaction so concurrent PUTs cannot
// clobber each other (TOCTOU). Updates uses a map so omitted fields are
// preserved (PUT semantics) and GORM skips zero-valued fields.
func (s *gormKnowledgeDocStore) Update(ctx context.Context, id uint, patch map[string]any, userID uint) (*models.KnowledgeDoc, error) {
	var out *models.KnowledgeDoc
	err := s.db.WithContext(withTimeout(ctx)).Transaction(func(tx *gorm.DB) error {
		var existing models.KnowledgeDoc
		q := tx.Model(&models.KnowledgeDoc{})
		if userID != 0 {
			q = q.Where("user_id = ?", userID)
		}
		if err := q.First(&existing, id).Error; err != nil {
			return err
		}
		for k, v := range patch {
			switch k {
			case "title":
				if str, ok := v.(string); ok {
					existing.Title = str
				}
			case "path":
				if str, ok := v.(string); ok {
					existing.Path = str
				}
			case "content":
				if str, ok := v.(string); ok {
					existing.Content = str
				}
			case "tags":
				if str, ok := v.(string); ok {
					existing.Tags = str
				}
			case "doc_type":
				if str, ok := v.(string); ok {
					existing.DocType = str
				}
			}
		}
		if err := tx.Save(&existing).Error; err != nil {
			return err
		}
		out = &existing
		return nil
	})
	return out, err
}

// Delete uses Unscoped so a future soft-delete column on the model does
// not silently change behavior. RowsAffected is returned for the handler
// to distinguish 404 from 200.
func (s *gormKnowledgeDocStore) Delete(ctx context.Context, id uint, userID uint) (int64, error) {
	q := s.db.WithContext(withTimeout(ctx)).Unscoped()
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	res := q.Delete(&models.KnowledgeDoc{}, id)
	return res.RowsAffected, res.Error
}
