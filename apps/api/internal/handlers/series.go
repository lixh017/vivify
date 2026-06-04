package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/opc/api/internal/models"
)

// SeriesStore is the persistence contract the SeriesHandler needs. It is
// intentionally narrow so tests can supply lightweight fakes and so the
// handler is not coupled to GORM. The interface is defined where it is
// consumed, per the "accept interfaces, return structs" Go idiom.
type SeriesStore interface {
	Create(ctx context.Context, s *models.Series) error
	List(ctx context.Context, filter SeriesFilter, page PageRequest) ([]models.Series, int64, error)
	Get(ctx context.Context, id uint, userID uint) (*models.Series, error)
	Update(ctx context.Context, id uint, patch map[string]any, userID uint) (*models.Series, error)
	Delete(ctx context.Context, id uint, userID uint) (int64, error)
}

// SeriesFilter narrows List results. Zero value means "no filter".
// IPID is a small QoL addition: a list-by-ip query is the common case
// once the IPProfile model lands in Phase 2.
type SeriesFilter struct {
	UserID uint
	IPID   uint
}

// SeriesHandler exposes the series REST surface. The store field is an
// interface so we can swap in fakes in unit tests; production wiring
// passes the *gormSeriesStore adapter defined in this file.
//
// Series is the smallest entity in the system (3 fields, no timestamps),
// so the handler is intentionally lean — no validation hooks, no enum
// guards. The pattern is the same as the other handlers; only the
// surface area is smaller.
type SeriesHandler struct {
	store  SeriesStore
	logger *slog.Logger
}

// NewSeriesHandler wires a SeriesHandler backed by the given *gorm.DB.
// This is the primary constructor used by main.go and by tests that
// want a real (in-memory) database.
func NewSeriesHandler(db *gorm.DB) *SeriesHandler {
	return NewSeriesHandlerWithStore(newGormSeriesStore(db), nil)
}

// NewSeriesHandlerWithStore wires a SeriesHandler against the given
// store and logger. A nil logger falls back to slog.Default(). Used by
// tests that want to inject a fake store.
func NewSeriesHandlerWithStore(store SeriesStore, logger *slog.Logger) *SeriesHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SeriesHandler{store: store, logger: logger}
}

// RegisterRoutes attaches the CRUD routes for /series to the given
// router. The handler owns its own URL space so main.go and tests do
// not duplicate wiring.
func (h *SeriesHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/series", h.Create)
	r.GET("/series", h.List)
	r.GET("/series/:id", h.Get)
	r.PUT("/series/:id", h.Update)
	r.DELETE("/series/:id", h.Delete)
}

// Create — POST /series
func (h *SeriesHandler) Create(c *gin.Context) {
	var s models.Series
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
	s.UserID = UserIDFromContext(c)
	if err := h.store.Create(c.Request.Context(), &s); err != nil {
		h.logger.Error("create series failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create series"})
		return
	}
	c.JSON(http.StatusCreated, s)
}

// List — GET /series?ip_id=&limit=&offset=
//
// Order is by updated_at desc (newest touch first) so the listing
// matches the other handlers. id is the secondary key so ties resolve
// deterministically across pages.
func (h *SeriesHandler) List(c *gin.Context) {
	filter := SeriesFilter{UserID: UserIDFromContext(c)}
	if v := c.Query("ip_id"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ip_id must be a positive integer"})
			return
		}
		filter.IPID = uint(n)
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
		h.logger.Error("list series failed", "err", err.Error(), "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list series"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"total":  total,
		"limit":  page.Limit,
		"offset": page.Offset,
	})
}

// Get — GET /series/:id
func (h *SeriesHandler) Get(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	s, err := h.store.Get(c.Request.Context(), id, UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "series not found"})
			return
		}
		h.logger.Error("get series failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get series"})
		return
	}
	c.JSON(http.StatusOK, s)
}

// Update — PUT /series/:id. Accepts a partial-update map so omitted
// fields do not silently clobber existing values (the classic
// ShouldBindJSON-into-existing-struct pitfall). The store performs the
// lookup + save in a transaction to avoid TOCTOU races.
func (h *SeriesHandler) Update(c *gin.Context) {
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

	s, err := h.store.Update(c.Request.Context(), id, patch, UserIDFromContext(c))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "series not found"})
			return
		}
		h.logger.Error("update series failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update series"})
		return
	}
	c.JSON(http.StatusOK, s)
}

// Delete — DELETE /series/:id. Returns 200 with {deleted: id} on
// success, 404 if the row did not exist.
func (h *SeriesHandler) Delete(c *gin.Context) {
	id, ok := parsePositiveID(c)
	if !ok {
		return
	}
	rows, err := h.store.Delete(c.Request.Context(), id, UserIDFromContext(c))
	if err != nil {
		h.logger.Error("delete series failed", "err", err.Error(), "id", id, "request_id", c.GetString("request_id"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete series"})
		return
	}
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "series not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// gormSeriesStore is the production SeriesStore backed by GORM. It
// binds every call to a bounded context so a slow query cannot outlive
// the request.
type gormSeriesStore struct {
	db *gorm.DB
}

func newGormSeriesStore(db *gorm.DB) *gormSeriesStore { return &gormSeriesStore{db: db} }

func (s *gormSeriesStore) Create(ctx context.Context, sr *models.Series) error {
	return s.db.WithContext(withTimeout(ctx)).Create(sr).Error
}

func (s *gormSeriesStore) List(ctx context.Context, f SeriesFilter, p PageRequest) ([]models.Series, int64, error) {
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.Series{}).Order(
		clause.OrderByColumn{Column: clause.Column{Name: "updated_at"}, Desc: true},
	).Order(clause.OrderByColumn{Column: clause.Column{Name: "id"}, Desc: false})
	if f.UserID != 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.IPID != 0 {
		q = q.Where("ip_id = ?", f.IPID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.Series
	if err := q.Limit(p.Limit).Offset(p.Offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *gormSeriesStore) Get(ctx context.Context, id uint, userID uint) (*models.Series, error) {
	if err := requireUserID(userID); err != nil {
		return nil, err
	}
	var sr models.Series
	q := s.db.WithContext(withTimeout(ctx)).Model(&models.Series{}).Where("user_id = ?", userID)
	if err := q.First(&sr, id).Error; err != nil {
		return nil, err
	}
	return &sr, nil
}

// Update wraps the read+write in a transaction so concurrent PUTs
// cannot clobber each other (TOCTOU). Updates uses a map so omitted
// fields are preserved (PUT semantics) and GORM skips zero-valued
// fields.
func (s *gormSeriesStore) Update(ctx context.Context, id uint, patch map[string]any, userID uint) (*models.Series, error) {
	if err := requireUserID(userID); err != nil {
		return nil, err
	}
	var out *models.Series
	err := s.db.WithContext(withTimeout(ctx)).Transaction(func(tx *gorm.DB) error {
		var existing models.Series
		q := tx.Model(&models.Series{}).Where("user_id = ?", userID)
		if err := q.First(&existing, id).Error; err != nil {
			return err
		}
		for k, v := range patch {
			switch k {
			case "name":
				if str, ok := v.(string); ok {
					existing.Name = str
				}
			case "description":
				if str, ok := v.(string); ok {
					existing.Description = str
				}
			case "ip_id":
				switch v := v.(type) {
				case float64:
					existing.IPID = uint(v)
				case int:
					existing.IPID = uint(v)
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

// Delete uses Unscoped so a future soft-delete column on the model
// does not silently change behavior. RowsAffected is returned for the
// handler to distinguish 404 from 200. userID is required (see
// requireUserID).
func (s *gormSeriesStore) Delete(ctx context.Context, id uint, userID uint) (int64, error) {
	if err := requireUserID(userID); err != nil {
		return 0, err
	}
	q := s.db.WithContext(withTimeout(ctx)).Unscoped().Where("user_id = ?", userID)
	res := q.Delete(&models.Series{}, id)
	return res.RowsAffected, res.Error
}
