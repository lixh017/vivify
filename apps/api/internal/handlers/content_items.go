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
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/opc/api/internal/models"
)

// parseTimeString accepts the common wire formats for *time.Time fields
// (RFC3339 with timezone, RFC3339Nano, and the SQL datetime shape
// "2006-01-02 15:04:05") so clients can post timestamps in whatever
// shape their framework hands them. The result is a *time.Time in UTC.
func parseTimeString(s string) (*time.Time, error) {
	if s == "" {
		return nil, errors.New("scheduled/published timestamp is empty")
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			tt := t.UTC()
			return &tt, nil
		}
	}
	return nil, errors.New("invalid timestamp format; expected RFC3339 or 'YYYY-MM-DD HH:MM:SS'")
}

// ContentItemStore is the persistence contract the ContentItemHandler
// needs. It is intentionally narrow so tests can supply lightweight fakes
// and so the handler is not coupled to GORM. The interface is defined
// where it is consumed, per the "accept interfaces, return structs" Go
// idiom.
//
// Phase 2 adds userID to Get/Update/Delete and UserID to the filter
// so reads and writes are scoped to the caller's rows.
type ContentItemStore interface {
	Create(ctx context.Context, c *models.ContentItem) error
	List(ctx context.Context, filter ContentItemFilter, page PageRequest) ([]models.ContentItem, int64, error)
	Get(ctx context.Context, id uint, userID uint) (*models.ContentItem, error)
	Update(ctx context.Context, id uint, patch map[string]any, userID uint) (*models.ContentItem, error)
	Delete(ctx context.Context, id uint, userID uint) (int64, error)
}

// ContentItemFilter narrows List results. Zero value means "no filter".
type ContentItemFilter struct {
	UserID   uint
	ScriptID uint
	Platform string
}

// ContentItemHandler exposes the content item REST surface. The store
// field is an interface so we can swap in fakes in unit tests; production
// wiring passes the *gormContentItemStore adapter defined in this file.
type ContentItemHandler struct {
	store  ContentItemStore
	logger *slog.Logger
}

// NewContentItemHandler wires a ContentItemHandler backed by the given
// *gorm.DB. This is the primary constructor used by main.go and by tests
// that want a real (in-memory) database. The store field is an interface
// so callers who need to swap in fakes can use
// NewContentItemHandlerWithStore.
func NewContentItemHandler(db *gorm.DB) *ContentItemHandler {
	return NewContentItemHandlerWithStore(newGormContentItemStore(db), nil)
}

// NewContentItemHandlerWithStore wires a ContentItemHandler against the
// given store and logger. A nil logger falls back to slog.Default(). Used
// by tests that want to inject a fake store.
func NewContentItemHandlerWithStore(store ContentItemStore, logger *slog.Logger) *ContentItemHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ContentItemHandler{store: store, logger: logger}
}

// RegisterRoutes attaches the CRUD routes for /content-items to the
// given router. The handler owns its own URL space so main.go and tests
// do not duplicate wiring.
func (h *ContentItemHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/content-items", h.Create)
	r.GET("/content-items", h.List)
	r.GET("/content-items/:id", h.Get)
	r.PUT("/content-items/:id", h.Update)
	r.DELETE("/content-items/:id", h.Delete)
}

// Create — POST /content-items
func (h *ContentItemHandler) Create(c *gin.Context) {
	var ci models.ContentItem
	if err := c.ShouldBindJSON(&ci); err != nil {
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
	// Stamp the owner. The body never carries a user_id.
	ci.UserID = UserIDFromContext(c)
	if err := h.store.Create(c.Request.Context(), &ci); err != nil {
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("create content item failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create content item"})
		return
	}
	c.JSON(http.StatusCreated, ci)
}

// List — GET /content-items?script_id=&platform=&limit=&offset=
func (h *ContentItemHandler) List(c *gin.Context) {
	filter := ContentItemFilter{UserID: UserIDFromContext(c)}
	if v := c.Query("script_id"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "script_id must be a positive integer"})
			return
		}
		filter.ScriptID = uint(n)
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
		h.logger.Error("list content items failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list content items"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  page.Limit,
		"offset": page.Offset,
	})
}

// Get — GET /content-items/:id
func (h *ContentItemHandler) Get(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	ci, err := h.store.Get(c.Request.Context(), id, UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "content item not found"})
			return
		}
		h.logger.Error("get content item failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get content item"})
		return
	}
	c.JSON(http.StatusOK, ci)
}

// Update — PUT /content-items/:id. Accepts a partial-update map so
// omitted fields do not silently clobber existing values (the classic
// ShouldBindJSON-into-existing-struct pitfall). The store performs the
// lookup + save in a transaction to avoid TOCTOU races.
func (h *ContentItemHandler) Update(c *gin.Context) {
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

	ci, err := h.store.Update(c.Request.Context(), id, patch, UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "content item not found"})
			return
		}
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("update content item failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update content item"})
		return
	}
	c.JSON(http.StatusOK, ci)
}

// Delete — DELETE /content-items/:id. Returns 200 with {deleted: id} on
// success, 404 if the row did not exist. (We diverge from the topic
// handler's 204 contract because the content item tests assert on the
// body.)
func (h *ContentItemHandler) Delete(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	rows, err := h.store.Delete(c.Request.Context(), id, UserIDFromContext(c))
	if err != nil {
		h.logger.Error("delete content item failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete content item"})
		return
	}
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "content item not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// gormContentItemStore is the production ContentItemStore backed by
// GORM. It binds every call to a bounded context so a slow query cannot
// outlive the request.
type gormContentItemStore struct {
	db *gorm.DB
}

func newGormContentItemStore(db *gorm.DB) *gormContentItemStore {
	return &gormContentItemStore{db: db}
}

func (s *gormContentItemStore) Create(ctx context.Context, ci *models.ContentItem) error {
	return s.db.WithContext(withTimeout(ctx)).Create(ci).Error
}

func (s *gormContentItemStore) List(ctx context.Context, f ContentItemFilter, p PageRequest) ([]models.ContentItem, int64, error) {
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.ContentItem{}).Order(
		clause.OrderByColumn{Column: clause.Column{Name: "created_at"}, Desc: true},
	)
	if f.UserID != 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.ScriptID != 0 {
		q = q.Where("script_id = ?", f.ScriptID)
	}
	if f.Platform != "" {
		q = q.Where("platform = ?", f.Platform)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.ContentItem
	if err := q.Limit(p.Limit).Offset(p.Offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *gormContentItemStore) Get(ctx context.Context, id uint, userID uint) (*models.ContentItem, error) {
	var ci models.ContentItem
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.ContentItem{})
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&ci, id).Error; err != nil {
		return nil, err
	}
	return &ci, nil
}

// Update wraps the read+write in a transaction so concurrent PUTs cannot
// clobber each other (TOCTOU). Updates uses a map so omitted fields are
// preserved (PUT semantics) and GORM skips zero-valued fields. Nullable
// *time.Time fields are handled explicitly: nil clears the value, a
// non-nil value updates it.
func (s *gormContentItemStore) Update(ctx context.Context, id uint, patch map[string]any, userID uint) (*models.ContentItem, error) {
	var out *models.ContentItem
	err := s.db.WithContext(withTimeout(ctx)).Transaction(func(tx *gorm.DB) error {
		var existing models.ContentItem
		q := tx.Model(&models.ContentItem{})
		if userID != 0 {
			q = q.Where("user_id = ?", userID)
		}
		if err := q.First(&existing, id).Error; err != nil {
			return err
		}
		for k, v := range patch {
			switch k {
			case "script_id":
				switch v := v.(type) {
				case float64:
					existing.ScriptID = uint(v)
				case int:
					existing.ScriptID = uint(v)
				}
			case "platform":
				if str, ok := v.(string); ok {
					existing.Platform = str
				}
			case "scheduled_at":
				switch v := v.(type) {
				case nil:
					existing.ScheduledAt = nil
				case string:
					t, err := parseTimeString(v)
					if err != nil {
						return err
					}
					existing.ScheduledAt = t
				}
			case "published_at":
				switch v := v.(type) {
				case nil:
					existing.PublishedAt = nil
				case string:
					t, err := parseTimeString(v)
					if err != nil {
						return err
					}
					existing.PublishedAt = t
				}
			case "platform_url":
				if str, ok := v.(string); ok {
					existing.PlatformURL = str
				}
			case "performance_metrics":
				if str, ok := v.(string); ok {
					existing.PerformanceMetrics = str
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
func (s *gormContentItemStore) Delete(ctx context.Context, id uint, userID uint) (int64, error) {
	q := s.db.WithContext(withTimeout(ctx)).Unscoped()
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	res := q.Delete(&models.ContentItem{}, id)
	return res.RowsAffected, res.Error
}
