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

// ScriptStore is the persistence contract the ScriptHandler needs. It is
// intentionally narrow so tests can supply lightweight fakes and so the
// handler is not coupled to GORM. The interface is defined where it is
// consumed, per the "accept interfaces, return structs" Go idiom.
//
// Phase 2 adds userID to Get/Update/Delete and UserID to the filter
// so reads and writes are scoped to the caller's rows.
type ScriptStore interface {
	Create(ctx context.Context, s *models.Script) error
	List(ctx context.Context, filter ScriptFilter, page PageRequest) ([]models.Script, int64, error)
	Get(ctx context.Context, id uint, userID uint) (*models.Script, error)
	Update(ctx context.Context, id uint, patch map[string]any, userID uint) (*models.Script, error)
	Delete(ctx context.Context, id uint, userID uint) (int64, error)
}

// ScriptFilter narrows List results. Zero value means "no filter".
type ScriptFilter struct {
	UserID   uint
	TopicID  uint
	Platform string
}

// ScriptHandler exposes the script REST surface. The store field is an
// interface so we can swap in fakes in unit tests; production wiring passes
// the *gormScriptStore adapter defined in this file.
type ScriptHandler struct {
	store  ScriptStore
	logger *slog.Logger
}

// NewScriptHandler wires a ScriptHandler backed by the given *gorm.DB. This
// is the primary constructor used by main.go and by tests that want a
// real (in-memory) database. The store field is an interface so callers
// who need to swap in fakes can use NewScriptHandlerWithStore.
func NewScriptHandler(db *gorm.DB) *ScriptHandler {
	return NewScriptHandlerWithStore(newGormScriptStore(db), nil)
}

// NewScriptHandlerWithStore wires a ScriptHandler against the given store
// and logger. A nil logger falls back to slog.Default(). Used by tests
// that want to inject a fake store.
func NewScriptHandlerWithStore(store ScriptStore, logger *slog.Logger) *ScriptHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ScriptHandler{store: store, logger: logger}
}

// NewScriptHandlerFromGorm is kept for parity with the topic handler. It
// is equivalent to NewScriptHandler(db).
func NewScriptHandlerFromGorm(db *gorm.DB, logger *slog.Logger) *ScriptHandler {
	return NewScriptHandlerWithStore(newGormScriptStore(db), logger)
}

// RegisterRoutes attaches the CRUD routes for /scripts to the given
// router. The handler owns its own URL space so main.go and tests do not
// duplicate wiring.
func (h *ScriptHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/scripts", h.Create)
	r.GET("/scripts", h.List)
	r.GET("/scripts/:id", h.Get)
	r.PUT("/scripts/:id", h.Update)
	r.DELETE("/scripts/:id", h.Delete)
}

// Create — POST /scripts
func (h *ScriptHandler) Create(c *gin.Context) {
	var s models.Script
	if err := c.ShouldBindJSON(&s); err != nil {
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
	// Stamp the owner from the auth context — never trust the request body.
	s.UserID = UserIDFromContext(c)
	if err := h.store.Create(c.Request.Context(), &s); err != nil {
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("create script failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create script"})
		return
	}
	c.JSON(http.StatusCreated, s)
}

// List — GET /scripts?topic_id=&platform=&limit=&offset=
func (h *ScriptHandler) List(c *gin.Context) {
	filter := ScriptFilter{UserID: UserIDFromContext(c)}
	if v := c.Query("topic_id"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "topic_id must be a positive integer"})
			return
		}
		filter.TopicID = uint(n)
	}
	filter.Platform = strings.TrimSpace(c.Query("platform"))

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
		h.logger.Error("list scripts failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list scripts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  page.Limit,
		"offset": page.Offset,
	})
}

// Get — GET /scripts/:id
func (h *ScriptHandler) Get(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	s, err := h.store.Get(c.Request.Context(), id, UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "script not found"})
			return
		}
		h.logger.Error("get script failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get script"})
		return
	}
	c.JSON(http.StatusOK, s)
}

// Update — PUT /scripts/:id. Accepts a partial-update map so omitted
// fields do not silently clobber existing values (the classic
// ShouldBindJSON-into-existing-struct pitfall). The store performs the
// lookup + save in a transaction to avoid TOCTOU races.
func (h *ScriptHandler) Update(c *gin.Context) {
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

	s, err := h.store.Update(c.Request.Context(), id, patch, UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "script not found"})
			return
		}
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("update script failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update script"})
		return
	}
	c.JSON(http.StatusOK, s)
}

// Delete — DELETE /scripts/:id. Returns 200 with {deleted: id} on success,
// 404 if the row did not exist. (We deliberately diverge from the topic
// handler's 204 contract because the script tests assert on the body.)
func (h *ScriptHandler) Delete(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	rows, err := h.store.Delete(c.Request.Context(), id, UserIDFromContext(c))
	if err != nil {
		h.logger.Error("delete script failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete script"})
		return
	}
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "script not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// gormScriptStore is the production ScriptStore backed by GORM. It binds
// every call to a bounded context so a slow query cannot outlive the
// request.
type gormScriptStore struct {
	db *gorm.DB
}

func newGormScriptStore(db *gorm.DB) *gormScriptStore { return &gormScriptStore{db: db} }

func (s *gormScriptStore) Create(ctx context.Context, sc *models.Script) error {
	return s.db.WithContext(withTimeout(ctx)).Create(sc).Error
}

func (s *gormScriptStore) List(ctx context.Context, f ScriptFilter, p PageRequest) ([]models.Script, int64, error) {
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.Script{}).Order(
		clause.OrderByColumn{Column: clause.Column{Name: "created_at"}, Desc: true},
	)
	if f.UserID != 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.TopicID != 0 {
		q = q.Where("topic_id = ?", f.TopicID)
	}
	if f.Platform != "" {
		q = q.Where("platform = ?", f.Platform)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var scripts []models.Script
	if err := q.Limit(p.Limit).Offset(p.Offset).Find(&scripts).Error; err != nil {
		return nil, 0, err
	}
	return scripts, total, nil
}

func (s *gormScriptStore) Get(ctx context.Context, id uint, userID uint) (*models.Script, error) {
	var sc models.Script
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.Script{})
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&sc, id).Error; err != nil {
		return nil, err
	}
	return &sc, nil
}

// Update wraps the read+write in a transaction so concurrent PUTs cannot
// clobber each other (TOCTOU). Updates uses a map so omitted fields are
// preserved (PATCH semantics) and GORM skips zero-valued fields.
func (s *gormScriptStore) Update(ctx context.Context, id uint, patch map[string]any, userID uint) (*models.Script, error) {
	var out *models.Script
	err := s.db.WithContext(withTimeout(ctx)).Transaction(func(tx *gorm.DB) error {
		var existing models.Script
		q := tx.Model(&models.Script{})
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
			case "content":
				if str, ok := v.(string); ok {
					existing.Content = str
				}
			case "platform":
				if str, ok := v.(string); ok {
					existing.Platform = str
				}
			case "style_fingerprint":
				if str, ok := v.(string); ok {
					existing.StyleFingerprint = str
				}
			case "tags":
				if str, ok := v.(string); ok {
					existing.Tags = str
				}
			case "word_count":
				switch v := v.(type) {
				case float64:
					existing.WordCount = int(v)
				case int:
					existing.WordCount = v
				}
			case "topic_id":
				switch v := v.(type) {
				case float64:
					u := uint(v)
					existing.TopicID = u
				case int:
					existing.TopicID = uint(v)
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
func (s *gormScriptStore) Delete(ctx context.Context, id uint, userID uint) (int64, error) {
	q := s.db.WithContext(withTimeout(ctx)).Unscoped()
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	res := q.Delete(&models.Script{}, id)
	return res.RowsAffected, res.Error
}
